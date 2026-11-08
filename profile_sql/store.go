package profile_sql

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type queryer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type database interface {
	queryer
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

type Store struct{ db database }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{db: pool} }

const bannedSQL = `(u.banned OR EXISTS (SELECT 1 FROM banned_devices bd
WHERE bd.device_id=u.device_id AND u.device_id IS NOT NULL AND u.device_id<>''))`

type account struct {
	id      int
	player  string
	isAdmin bool
}

func readAccount(ctx context.Context, q queryer, player string, lock bool, viewer bool) (account, error) {
	var a account
	var banned bool
	sql := `SELECT u.id,u.player_id,u.is_admin,` + bannedSQL + ` FROM users u WHERE u.player_id=$1`
	if lock {
		sql += ` FOR NO KEY UPDATE OF u`
	}
	err := q.QueryRow(ctx, sql, player).Scan(&a.id, &a.player, &a.isAdmin, &banned)
	if errors.Is(err, pgx.ErrNoRows) {
		if viewer {
			return a, ErrUnauthorized
		}
		return a, ErrNotFound
	}
	if err != nil {
		return a, err
	}
	if banned {
		if viewer {
			return a, ErrForbidden
		}
		return a, ErrNotFound
	}
	return a, nil
}

const ownershipSQL = `SELECT cosmetic_id FROM user_cosmetics WHERE user_id=$1
UNION SELECT l.cosmetic_id FROM user_statuses s
JOIN profile_legacy_cosmetics l ON l.status=lower(btrim(s.status)) WHERE s.user_id=$1`

func ownership(ctx context.Context, q queryer, userID int) (map[string]bool, error) {
	rows, err := q.Query(ctx, ownershipSQL, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	owned := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		owned[id] = true
	}
	return owned, rows.Err()
}

// Фиксированные идентификаторы исключают SQL-инъекции через данные запроса.
func cosmeticAccessSQL(alias string) string {
	return `(u.is_admin OR ` + alias + `.source='starter'
OR (NOT ` + alias + `.admin_only AND (
EXISTS (SELECT 1 FROM user_cosmetics uc WHERE uc.user_id=u.id AND uc.cosmetic_id=` + alias + `.id)
OR EXISTS (SELECT 1 FROM user_statuses us JOIN profile_legacy_cosmetics lc ON lc.status=lower(btrim(us.status))
WHERE us.user_id=u.id AND lc.cosmetic_id=` + alias + `.id))))`
}

func publicColumns(thumb bool) string {
	avatar := "p.avatar"
	if thumb {
		avatar = "p.avatar_thumb"
	}
	return `u.id,u.player_id,u.username,COALESCE(p.bio,''),` + avatar + `,
CASE WHEN ct.kind='theme' AND ` + cosmeticAccessSQL("ct") + ` THEN ct.id ELSE 'theme-tide' END,
CASE WHEN ce.kind='emblem' AND ` + cosmeticAccessSQL("ce") + ` THEN ce.id ELSE 'emblem-none' END,
CASE WHEN cp.kind='pattern' AND ` + cosmeticAccessSQL("cp") + ` THEN cp.id ELSE 'pattern-orbit' END,
u.is_admin,u.created_at,COALESCE(p.show_balance,true),
CASE WHEN COALESCE(p.show_balance,true) THEN w.balance::double precision ELSE NULL END,
f.requester_id,f.state`
}

const publicJoins = ` FROM users u
LEFT JOIN user_profiles p ON p.user_id=u.id
LEFT JOIN wallets w ON w.user_id=u.id
LEFT JOIN profile_cosmetic_catalog ct ON ct.id=COALESCE(p.theme_id,'theme-tide')
LEFT JOIN profile_cosmetic_catalog ce ON ce.id=COALESCE(p.emblem_id,'emblem-none')
LEFT JOIN profile_cosmetic_catalog cp ON cp.id=COALESCE(p.pattern_id,'pattern-orbit')
LEFT JOIN community_friendships f ON f.user_low=LEAST(u.id,$1::integer) AND f.user_high=GREATEST(u.id,$1::integer)`

type scanner interface{ Scan(...any) error }

func scanPublic(row scanner, viewerID int, rank *int) (PublicProfile, error) {
	var p PublicProfile
	var id int
	var avatar []byte
	var requester *int
	var state *string
	args := []any{&id, &p.PlayerID, &p.Username, &p.Bio, &avatar, &p.ThemeID, &p.EmblemID, &p.PatternID,
		&p.IsAdmin, &p.CreatedAt, &p.ShowBalance, &p.Balance, &requester, &state}
	if rank != nil {
		args = append(args, rank)
	}
	if err := row.Scan(args...); err != nil {
		return p, err
	}
	if len(avatar) > 0 {
		p.AvatarURL = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(avatar)
	}
	if !p.ShowBalance {
		p.Balance = nil // Скрываем баланс даже при ошибке в SQL-проекции.
	}
	p.FriendState = relationshipState(viewerID, id, requester, state)
	return p, nil
}

func publicProfile(ctx context.Context, q queryer, viewerID int, player string) (PublicProfile, error) {
	row := q.QueryRow(ctx, `SELECT `+publicColumns(false)+publicJoins+` WHERE u.player_id=$2 AND NOT `+bannedSQL, viewerID, player)
	p, err := scanPublic(row, viewerID, nil)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

func ownState(ctx context.Context, q queryer, a account) (OwnState, error) {
	var s OwnState
	p, err := publicProfile(ctx, q, a.id, a.player)
	if err != nil {
		return s, err
	}
	owned, err := ownership(ctx, q, a.id)
	if err != nil {
		return s, err
	}
	err = q.QueryRow(ctx, `SELECT lavx_balance::double precision FROM wallets WHERE user_id=$1`, a.id).Scan(&s.LavxBalance)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, ErrWalletMissing
	}
	if err != nil {
		return s, err
	}
	p.ThemeID = displayCosmetic(p.ThemeID, "theme", DefaultTheme, owned, p.IsAdmin)
	p.EmblemID = displayCosmetic(p.EmblemID, "emblem", DefaultEmblem, owned, p.IsAdmin)
	p.PatternID = displayCosmetic(p.PatternID, "pattern", DefaultPattern, owned, p.IsAdmin)
	s.Profile = p
	s.Catalog = CatalogFor(owned, p.IsAdmin)
	return s, nil
}

func (s *Store) Own(ctx context.Context, player string) (OwnState, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return OwnState{}, err
	}
	defer tx.Rollback(ctx)
	a, err := readAccount(ctx, tx, player, false, true)
	if err != nil {
		return OwnState{}, err
	}
	result, err := ownState(ctx, tx, a)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

func (s *Store) View(ctx context.Context, viewer, target string) (PublicProfile, error) {
	if err := ValidatePlayerID(target); err != nil {
		return PublicProfile{}, err
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return PublicProfile{}, err
	}
	defer tx.Rollback(ctx)
	a, err := readAccount(ctx, tx, viewer, false, true)
	if err != nil {
		return PublicProfile{}, err
	}
	p, err := publicProfile(ctx, tx, a.id, target)
	if err != nil {
		return p, err
	}
	return p, tx.Commit(ctx)
}

func ensureProfile(ctx context.Context, tx pgx.Tx, userID int) error {
	_, err := tx.Exec(ctx, `INSERT INTO user_profiles(user_id) VALUES ($1) ON CONFLICT(user_id) DO NOTHING`, userID)
	return err
}

func (s *Store) mutate(ctx context.Context, player string, fn func(pgx.Tx, account) error) (OwnState, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return OwnState{}, err
	}
	defer tx.Rollback(ctx)
	a, err := readAccount(ctx, tx, player, true, true)
	if err != nil {
		return OwnState{}, err
	}
	if err = fn(tx, a); err != nil {
		return OwnState{}, err
	}
	result, err := ownState(ctx, tx, a)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

func (s *Store) Update(ctx context.Context, player string, in UpdateInput) (OwnState, error) {
	if err := ValidateUpdate(in); err != nil {
		return OwnState{}, err
	}
	return s.mutate(ctx, player, func(tx pgx.Tx, a account) error {
		var currentName string
		if err := tx.QueryRow(ctx, `SELECT username FROM users WHERE id=$1`, a.id).Scan(&currentName); err != nil {
			return err
		}
		if in.Username != currentName {
			if !validText(in.Username, 3, 24) {
				return ErrInvalid
			}
			// Старые имена могут совпадать; уникальность проверяется только при смене.
			if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(1735390032)`); err != nil {
				return err
			}
			var taken bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id<>$1
AND (lower(username)=lower($2) OR player_id=$2))`, a.id, in.Username).Scan(&taken); err != nil {
				return err
			}
			if taken || (ValidatePlayerID(in.Username) == nil && in.Username != player) {
				return ErrUsernameTaken
			}
			if _, err := tx.Exec(ctx, `UPDATE users SET username=$2 WHERE id=$1`, a.id, in.Username); err != nil {
				return err
			}
		}
		if err := ensureProfile(ctx, tx, a.id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE user_profiles SET bio=$2,show_balance=$3,updated_at=now() WHERE user_id=$1`, a.id, in.Bio, in.ShowBalance)
		return err
	})
}

func (s *Store) Equip(ctx context.Context, player string, in EquipInput) (OwnState, error) {
	if err := ValidateEquip(in); err != nil {
		return OwnState{}, err
	}
	return s.mutate(ctx, player, func(tx pgx.Tx, a account) error {
		owned, err := ownership(ctx, tx, a.id)
		if err != nil {
			return err
		}
		for _, id := range []string{in.ThemeID, in.EmblemID, in.PatternID} {
			if !HasCosmetic(id, owned, a.isAdmin) {
				return ErrNotOwned
			}
		}
		if err := ensureProfile(ctx, tx, a.id); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE user_profiles SET theme_id=$2,emblem_id=$3,pattern_id=$4,updated_at=now() WHERE user_id=$1`,
			a.id, in.ThemeID, in.EmblemID, in.PatternID)
		return err
	})
}

const lockWalletSQL = `SELECT lavx_balance::double precision FROM wallets WHERE user_id=$1 FOR UPDATE`
const debitWalletSQL = `UPDATE wallets SET lavx_balance=lavx_balance-$2,updated_at=now()
WHERE user_id=$1 AND lavx_balance >= $2`

// Вставка до списания исключает оплату уже выданного предмета даже при гонке.
func purchaseTx(ctx context.Context, tx pgx.Tx, a account, c Cosmetic) (bool, error) {
	if c.AdminOnly || c.Source != "shop" || c.Price <= 0 {
		return false, ErrNotPurchasable
	}
	var balance float64
	err := tx.QueryRow(ctx, lockWalletSQL, a.id).Scan(&balance)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrWalletMissing
	}
	if err != nil {
		return false, err
	}
	owned, err := ownership(ctx, tx, a.id)
	if err != nil {
		return false, err
	}
	if HasCosmetic(c.ID, owned, a.isAdmin) {
		return false, nil
	}
	added, err := GrantCosmeticTx(ctx, tx, a.id, c.ID, "shop")
	if err != nil || !added {
		return false, err
	}
	// Проверяем баланс при списании в NUMERIC, чтобы не зависеть от округления float.
	tag, err := tx.Exec(ctx, debitWalletSQL, a.id, c.Price)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() != 1 {
		return false, ErrInsufficientLavx
	}
	return true, nil
}

func (s *Store) Purchase(ctx context.Context, player, id string) (OwnState, error) {
	c, ok := GetCosmetic(id)
	if !ok {
		return OwnState{}, ErrInvalid
	}
	if c.AdminOnly || c.Source != "shop" || c.Price <= 0 {
		return OwnState{}, ErrNotPurchasable
	}
	return s.mutate(ctx, player, func(tx pgx.Tx, a account) error {
		_, err := purchaseTx(ctx, tx, a, c)
		return err
	})
}

// Метаданные очищает profile_http.ProcessAvatar, поэтому здесь допустим только его результат.
func (s *Store) SetAvatar(ctx context.Context, player string, avatar, thumb []byte) (OwnState, error) {
	if (len(avatar) == 0) != (len(thumb) == 0) || len(avatar) > 96*1024 || len(thumb) > 12*1024 {
		return OwnState{}, ErrInvalid
	}
	for _, b := range [][]byte{avatar, thumb} {
		if len(b) > 0 && (len(b) < 3 || b[0] != 0xff || b[1] != 0xd8 || b[2] != 0xff) {
			return OwnState{}, ErrInvalid
		}
	}
	if len(avatar) == 0 {
		avatar, thumb = nil, nil
	}
	return s.mutate(ctx, player, func(tx pgx.Tx, a account) error {
		if err := ensureProfile(ctx, tx, a.id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE user_profiles SET avatar=$2,avatar_thumb=$3,updated_at=now() WHERE user_id=$1`, a.id, avatar, thumb)
		return err
	})
}

func escapeSearch(q string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
}
