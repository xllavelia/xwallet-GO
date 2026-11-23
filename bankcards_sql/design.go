package bankcards_sql

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image/jpeg"
	"io"
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
	customMaxDimension  = 4096
)

var (
	ErrInvalidDesign  = errors.New("design is not available")
	ErrCustomInvalid  = errors.New("custom design must contain one eligible designKey or JPEG photo within upload limits")
	ErrDesignNotOwned = errors.New("card design is not owned")
	ErrVoucherInvalid = errors.New("custom voucher not found or already used")
)

// CardCustomDesign — доступный пресет или одна фотография на всю карточку.
type CardCustomDesign struct {
	DesignKey string `json:"designKey,omitempty"`
	Photo     string `json:"photo,omitempty"`
}

// DesignChoice — выбор оформления: базовый дизайн, разблокированный или кастомный.
type DesignChoice struct {
	DesignID int
	// DesignIDSet отличает явный designId:0 от отсутствующего базового дизайна.
	DesignIDSet bool
	DesignKey   string
	Custom      *CardCustomDesign
}

func validateCustomImage(dataURL string) (string, error) {
	const prefix = "data:image/jpeg;base64,"
	if !strings.HasPrefix(dataURL, prefix) {
		return "", ErrCustomInvalid
	}
	payload := strings.TrimPrefix(dataURL, prefix)
	if len(payload) > base64.StdEncoding.EncodedLen(customMaxImageBytes) || strings.ContainsAny(payload, " \t\r\n") {
		return "", ErrCustomInvalid
	}
	data, err := base64.StdEncoding.Strict().DecodeString(payload)
	if err != nil || len(data) == 0 || len(data) > customMaxImageBytes {
		return "", ErrCustomInvalid
	}
	// Размеры проверяются до выделения памяти под пиксели.
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > customMaxDimension || cfg.Height > customMaxDimension {
		return "", ErrCustomInvalid
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil || img.Bounds().Dx() != cfg.Width || img.Bounds().Dy() != cfg.Height {
		return "", ErrCustomInvalid
	}
	return prefix + base64.StdEncoding.EncodeToString(data), nil
}

// ValidateCustomDesign проверяет пресет независимо от владения и роли пользователя.
func ValidateCustomDesign(custom *CardCustomDesign) (*CardCustomDesign, error) {
	if custom == nil {
		return nil, nil
	}
	if (custom.DesignKey == "") == (custom.Photo == "") {
		return nil, ErrCustomInvalid
	}
	if custom.DesignKey != "" {
		key, ok := profile_sql.NormalizeCardDesignKey(custom.DesignKey)
		if !ok {
			return nil, ErrCustomInvalid
		}
		design, _ := profile_sql.GetCosmetic(key)
		eligible := !design.AdminOnly && ((design.Source == "shop" && design.Price <= 50) ||
			(design.Source == "battlepass" && design.Track == "free"))
		if !eligible {
			return nil, ErrCustomInvalid
		}
		return &CardCustomDesign{DesignKey: key}, nil
	}
	photo, err := validateCustomImage(custom.Photo)
	if err != nil {
		return nil, err
	}
	return &CardCustomDesign{Photo: photo}, nil
}

// ValidateDesignChoice проверяет выбор оформления и доступ к разблокированным дизайнам.
func ValidateDesignChoice(choice DesignChoice, owned map[string]bool, isAdmin bool) (DesignChoice, error) {
	advanced := choice.DesignKey != "" || choice.Custom != nil
	if (choice.DesignKey != "" && choice.Custom != nil) || (advanced && (choice.DesignIDSet || choice.DesignID != 0)) {
		return choice, ErrInvalidDesign // Допустим только один способ оформления.
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
	// Старое оформление игнорируется без изменения сохранённого JSON.
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var custom *CardCustomDesign
	if err := decoder.Decode(&custom); err != nil {
		return nil, nil
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, nil
	}
	validated, err := ValidateCustomDesign(custom)
	if err != nil {
		return nil, nil
	}
	return validated, nil
}

// ChargeCustomDesign проверяет и оплачивает оформление главного кошелька.
func ChargeCustomDesign(ctx context.Context, pool *pgxpool.Pool, userID, voucherID int, custom *CardCustomDesign) (*CardCustomDesign, error) {
	custom, err := ValidateCustomDesign(custom)
	if err != nil {
		return nil, err
	}
	if custom == nil {
		return nil, ErrCustomInvalid
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var isAdmin bool
	if err := tx.QueryRow(ctx, `SELECT is_admin FROM users WHERE id=$1`, userID).Scan(&isAdmin); err != nil {
		return nil, err
	}
	if err := chargeCustomTx(ctx, tx, userID, isAdmin, voucherID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return custom, nil
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
	if choice.Custom != nil {
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
	if choice.Custom != nil {
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
