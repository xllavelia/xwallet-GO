package bankcards_sql

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xwallet-server/profile_sql"
	"xwallet-server/user_vouchers_sql"
)

const (
	// BaseDesignCount — базовые дизайны, доступные каждому: 0 — кошелёк, 1–3 — основные счета.
	BaseDesignCount = 4
	// CustomCardPriceLavx — цена платной кастомизации карточки для обычных игроков.
	CustomCardPriceLavx = 50.0
	customMaxImageBytes = 160 * 1024
	customMaxTotalBytes = 300 * 1024
)

var (
	ErrInvalidDesign  = errors.New("design is not available")
	ErrCustomInvalid  = errors.New("custom design contains invalid colors or images")
	ErrDesignNotOwned = errors.New("card design is not owned")
	ErrVoucherInvalid = errors.New("custom voucher not found or already used")
	ErrNFTNotOwned    = errors.New("nft is not owned")
)

var nftRarities = map[string]bool{
	"common": true, "uncommon": true, "rare": true, "epic": true, "mythic": true, "legendary": true,
}

const nftArtMaxBytes = 2048

// CardSide — одна из частей карточки: цвет и/или фотография.
type CardSide struct {
	Color string `json:"color,omitempty"`
	Image string `json:"image,omitempty"`
}

// CardNFTSkin — оформление карты рисунком NFT: арт слева, градиент редкости справа.
// Арт хранится снимком: проданный NFT остаётся на карте как оформление.
type CardNFTSkin struct {
	ItemID int64           `json:"itemId"`
	Name   string          `json:"name,omitempty"`
	Rarity string          `json:"rarity"`
	Art    json.RawMessage `json:"art"`
}

// CardCustomDesign — пользовательская карточка: левая 2/3 и правая 1/3.
// Link объединяет части: левая заливка и цвет текста действуют на всю карту.
type CardCustomDesign struct {
	NFT        *CardNFTSkin `json:"nft,omitempty"`
	Left       CardSide     `json:"left"`
	Right      CardSide     `json:"right"`
	Ink        string       `json:"ink,omitempty"`
	InkOpacity *float64     `json:"inkOpacity,omitempty"`
	Link       bool         `json:"link"`
}

// DesignChoice — выбор оформления: базовый дизайн, разблокированный или кастомный.
type DesignChoice struct {
	DesignID  int
	DesignKey string
	Custom    *CardCustomDesign
}

var hexColorPattern = regexp.MustCompile(`^#?([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// NormalizeHexColor приводит hex к виду #rrggbb; пустая строка остаётся пустой.
func NormalizeHexColor(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	match := hexColorPattern.FindStringSubmatch(value)
	if match == nil {
		return "", ErrCustomInvalid
	}
	hex := strings.ToLower(match[1])
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	return "#" + hex, nil
}

func validateCustomImage(dataURL string) (string, error) {
	const prefix = "data:image/jpeg;base64,"
	if dataURL == "" {
		return "", nil
	}
	if !strings.HasPrefix(dataURL, prefix) {
		return "", ErrCustomInvalid
	}
	payload := strings.TrimPrefix(dataURL, prefix)
	if strings.ContainsAny(payload, " \t\r\n") {
		return "", ErrCustomInvalid
	}
	if base64.StdEncoding.DecodedLen(len(payload)) > customMaxImageBytes+2 {
		return "", ErrCustomInvalid
	}
	return dataURL, nil
}

// validateNFTSkin проверяет снимок NFT-оформления; арт приходит с клиента.
func validateNFTSkin(skin *CardNFTSkin) (*CardNFTSkin, error) {
	if skin == nil {
		return nil, nil
	}
	if skin.ItemID <= 0 || !nftRarities[skin.Rarity] {
		return nil, ErrCustomInvalid
	}
	if len(skin.Art) == 0 || len(skin.Art) > nftArtMaxBytes || !json.Valid(skin.Art) {
		return nil, ErrCustomInvalid
	}
	name := strings.TrimSpace(skin.Name)
	if len(name) > 64 || strings.ContainsAny(name, "\x00\r\n\t") {
		return nil, ErrCustomInvalid
	}
	return &CardNFTSkin{ItemID: skin.ItemID, Name: name, Rarity: skin.Rarity, Art: skin.Art}, nil
}

// ValidateCustomDesign проверяет и нормализует кастомный дизайн.
func ValidateCustomDesign(custom *CardCustomDesign) (*CardCustomDesign, error) {
	if custom == nil {
		return nil, nil
	}
	if custom.InkOpacity != nil && (*custom.InkOpacity < 0 || *custom.InkOpacity > 1) {
		return nil, ErrCustomInvalid
	}
	out := &CardCustomDesign{Link: custom.Link, Ink: custom.Ink, InkOpacity: custom.InkOpacity}
	var err error
	if out.NFT, err = validateNFTSkin(custom.NFT); err != nil {
		return nil, err
	}
	if out.NFT != nil {
		// NFT-скин рисуется целиком: цвета, фото и текст не используются.
		out.Link, out.Ink, out.InkOpacity = false, "", nil
		out.Left, out.Right = CardSide{}, CardSide{}
		return out, nil
	}
	if out.Left.Color, err = NormalizeHexColor(custom.Left.Color); err != nil {
		return nil, err
	}
	if out.Right.Color, err = NormalizeHexColor(custom.Right.Color); err != nil {
		return nil, err
	}
	if out.Ink, err = NormalizeHexColor(custom.Ink); err != nil {
		return nil, err
	}
	if out.Left.Image, err = validateCustomImage(custom.Left.Image); err != nil {
		return nil, err
	}
	if out.Right.Image, err = validateCustomImage(custom.Right.Image); err != nil {
		return nil, err
	}
	if len(out.Left.Image)+len(out.Right.Image) > customMaxTotalBytes {
		return nil, ErrCustomInvalid
	}
	if out.Link {
		// В связанном режиме правая часть наследует левую.
		out.Right = CardSide{}
	}
	if out.Left.Color == "" && out.Left.Image == "" {
		return nil, ErrCustomInvalid
	}
	raw, err := json.Marshal(out)
	if err != nil || len(raw) > customMaxTotalBytes+2048 {
		return nil, ErrCustomInvalid
	}
	return out, nil
}

// ValidateDesignChoice проверяет выбор оформления и доступ к разблокированным дизайнам.
func ValidateDesignChoice(choice DesignChoice, owned map[string]bool, isAdmin bool) (DesignChoice, error) {
	advanced := choice.DesignKey != "" || choice.Custom != nil
	if advanced && choice.DesignID != 0 {
		return choice, ErrInvalidDesign // Ключ/кастом не сочетаются с явным базовым дизайном.
	}
	if !advanced && (choice.DesignID < 0 || choice.DesignID >= BaseDesignCount) {
		return choice, ErrInvalidDesign
	}
	var err error
	if choice.Custom, err = ValidateCustomDesign(choice.Custom); err != nil {
		return choice, err
	}
	if choice.DesignKey != "" {
		normalized, ok := profile_sql.NormalizeCardDesignKey(choice.DesignKey)
		if !ok {
			return choice, ErrInvalidDesign
		}
		if !profile_sql.HasCosmetic(normalized, owned, isAdmin) {
			return choice, ErrDesignNotOwned
		}
		choice.DesignKey = normalized
	}
	return choice, nil
}

func cardOwnership(ctx context.Context, tx pgx.Tx, userID int) (map[string]bool, bool, error) {
	var isAdmin bool
	if err := tx.QueryRow(ctx, `SELECT is_admin FROM users WHERE id=$1`, userID).Scan(&isAdmin); err != nil {
		return nil, false, err
	}
	rows, err := tx.Query(ctx, `SELECT cosmetic_id FROM user_cosmetics WHERE user_id=$1`, userID)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	owned := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, false, err
		}
		owned[id] = true
	}
	return owned, isAdmin, rows.Err()
}

// chargeCustomTx списывает цену кастомизации; админам — бесплатно, ваучер даёт скидку.
func chargeCustomTx(ctx context.Context, tx pgx.Tx, userID int, isAdmin bool, voucherID int) error {
	consume := func() (float64, error) {
		discount, err := user_vouchers_sql.ConsumeCustomVoucherTx(ctx, tx, userID, voucherID)
		if err != nil {
			return 0, ErrVoucherInvalid
		}
		return discount, nil
	}
	if isAdmin {
		if voucherID > 0 {
			_, err := consume()
			return err
		}
		return nil
	}
	price := CustomCardPriceLavx
	if voucherID > 0 {
		discount, err := consume()
		if err != nil {
			return err
		}
		price = price * (100 - discount) / 100
	}
	if price <= 0 {
		return nil
	}
	tag, err := tx.Exec(ctx, `UPDATE wallets SET lavx_balance=lavx_balance-$2,updated_at=now()
WHERE user_id=$1 AND lavx_balance >= $2`, userID, price)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrInsufficientFunds
	}
	return nil
}

// verifyNFTSkin проверяет, что оформление взято из собственного инвентаря.
func verifyNFTSkin(ctx context.Context, tx pgx.Tx, userID int, custom *CardCustomDesign) error {
	if custom == nil || custom.NFT == nil {
		return nil
	}
	var owned bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM nft_items WHERE id=$1 AND owner_id=$2)`,
		custom.NFT.ItemID, userID).Scan(&owned); err != nil {
		return err
	}
	if !owned {
		return ErrNFTNotOwned
	}
	return nil
}

// customIsPaid: NFT-скин бесплатен, платна только ручная кастомизация.
func customIsPaid(custom *CardCustomDesign) bool {
	return custom != nil && custom.NFT == nil
}

func customJSON(custom *CardCustomDesign) any {
	if custom == nil {
		return nil
	}
	raw, _ := json.Marshal(custom)
	return raw
}

func scanCustom(raw []byte) (*CardCustomDesign, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var custom CardCustomDesign
	if err := json.Unmarshal(raw, &custom); err != nil {
		return nil, err
	}
	return &custom, nil
}

// ChargeCustomDesign списывает цену кастомизации для кошелька, у которого нет отдельной карточки.
func ChargeCustomDesign(ctx context.Context, pool *pgxpool.Pool, userID, voucherID int) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var isAdmin bool
	if err := tx.QueryRow(ctx, `SELECT is_admin FROM users WHERE id=$1`, userID).Scan(&isAdmin); err != nil {
		return err
	}
	if err := chargeCustomTx(ctx, tx, userID, isAdmin, voucherID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// OpenCardEx открывает счёт с выбранным оформлением и оплатой кастомизации.
func OpenCardEx(ctx context.Context, pool *pgxpool.Pool, userID int, name string, choice DesignChoice, voucherID int) (Card, error) {
	name, err := ValidateOpenCard(name)
	if err != nil {
		return Card{}, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Card{}, err
	}
	defer tx.Rollback(ctx)

	owned, isAdmin, err := cardOwnership(ctx, tx, userID)
	if err != nil {
		return Card{}, err
	}
	choice, err = ValidateDesignChoice(choice, owned, isAdmin)
	if err != nil {
		return Card{}, err
	}
	if err := verifyNFTSkin(ctx, tx, userID, choice.Custom); err != nil {
		return Card{}, err
	}
	if customIsPaid(choice.Custom) {
		if err := chargeCustomTx(ctx, tx, userID, isAdmin, voucherID); err != nil {
			return Card{}, err
		}
	}

	card, err := openCardTx(ctx, tx, userID, name, choice)
	if err != nil {
		return Card{}, err
	}
	return card, tx.Commit(ctx)
}

// SetCardDesign меняет оформление существующего счёта.
func SetCardDesign(ctx context.Context, pool *pgxpool.Pool, userID, cardID int, choice DesignChoice, voucherID int) (Card, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Card{}, err
	}
	defer tx.Rollback(ctx)

	owned, isAdmin, err := cardOwnership(ctx, tx, userID)
	if err != nil {
		return Card{}, err
	}
	choice, err = ValidateDesignChoice(choice, owned, isAdmin)
	if err != nil {
		return Card{}, err
	}
	if err := lockOwnedCard(ctx, tx, userID, cardID); err != nil {
		return Card{}, err
	}
	if err := verifyNFTSkin(ctx, tx, userID, choice.Custom); err != nil {
		return Card{}, err
	}
	if customIsPaid(choice.Custom) {
		if err := chargeCustomTx(ctx, tx, userID, isAdmin, voucherID); err != nil {
			return Card{}, err
		}
	}

	var card Card
	var customRaw []byte
	err = tx.QueryRow(ctx, `
		UPDATE bank_cards SET design_id=$3, design_key=$4, custom=$5
		WHERE id=$1 AND user_id=$2
		RETURNING id, user_id, tier, name, design_id, design_key, custom, card_number, balance, is_active_for_trading, opened_at;
	`, cardID, userID, choice.DesignID, nullIfEmpty(choice.DesignKey), customJSON(choice.Custom)).Scan(
		&card.ID, &card.UserID, &card.Tier, &card.Name, &card.DesignID, &card.DesignKey, &customRaw, &card.CardNumber,
		&card.Balance, &card.IsActiveForTrading, &card.OpenedAt)
	if err != nil {
		return Card{}, err
	}
	if card.Custom, err = scanCustom(customRaw); err != nil {
		return Card{}, err
	}
	return card, tx.Commit(ctx)
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}
