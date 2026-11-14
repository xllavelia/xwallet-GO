package battlepass_sql

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var BPErrInvalidLevel = errors.New("invalid level")

type BPAdminClaim struct {
	Level int    `json:"level"`
	Track string `json:"track"`
}

type BPAdminState struct {
	Season       BPSeasonState  `json:"season"`
	XP           int64          `json:"xp"`
	Level        int            `json:"level"`
	LevelXP      int64          `json:"levelXp"`
	MaxLevel     int            `json:"maxLevel"`
	XPPerLevel   int            `json:"xpPerLevel"`
	PassOwned    bool           `json:"passOwned"`
	Claims       []BPAdminClaim `json:"claims"`
	PendingCases int            `json:"pendingCases"`
}

func bpAdminStateTx(ctx context.Context, tx pgx.Tx, userID int, season BPSeasonState) (*BPAdminState, error) {
	xp, err := bpEnsureProfileTx(ctx, tx, userID, season.ID)
	if err != nil {
		return nil, err
	}
	s := &BPAdminState{Season: season, XP: xp, MaxLevel: BPMaxLevel, XPPerLevel: BPXpPerLevel, Claims: []BPAdminClaim{}}
	s.Level, s.LevelXP = BPLevelForXP(xp)
	if err = tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM bp_passes WHERE user_id = $1 AND season_id = $2),
       (SELECT COUNT(*) FROM bp_case_tokens WHERE user_id = $1 AND season_id = $2 AND opened_at IS NULL)`,
		userID, season.ID).Scan(&s.PassOwned, &s.PendingCases); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
SELECT level, track FROM bp_claims WHERE user_id = $1 AND season_id = $2 ORDER BY level, track`, userID, season.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c BPAdminClaim
		if err = rows.Scan(&c.Level, &c.Track); err != nil {
			return nil, err
		}
		s.Claims = append(s.Claims, c)
	}
	return s, rows.Err()
}

func BPAdminLoad(ctx context.Context, pool *pgxpool.Pool, userID int) (*BPAdminState, error) {
	season, err := BPEnsureSeason(ctx, pool)
	if err != nil {
		return nil, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	s, err := bpAdminStateTx(ctx, tx, userID, season)
	if err != nil {
		return nil, err
	}
	return s, tx.Commit(ctx)
}

// BPAdminSetLevel переносит игрока на уровень, сохраняя прогресс внутри уровня.
// При понижении стираются отметки наград и неоткрытые кейсы с уровней выше нового,
// чтобы их можно было честно получить заново и интерфейс не видел «забранных» закрытых уровней.
func BPAdminSetLevel(ctx context.Context, pool *pgxpool.Pool, userID int, level int) (*BPAdminState, error) {
	if level < 1 || level > BPMaxLevel {
		return nil, BPErrInvalidLevel
	}
	season, err := BPEnsureSeason(ctx, pool)
	if err != nil {
		return nil, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	xp, err := bpEnsureProfileTx(ctx, tx, userID, season.ID)
	if err != nil {
		return nil, err
	}
	cur, levelXP := BPLevelForXP(xp)
	if cur == BPMaxLevel {
		levelXP = 0
	}
	newXP := int64((level - 1) * BPXpPerLevel)
	if level < BPMaxLevel {
		newXP += min(levelXP, int64(BPXpPerLevel-1))
	}
	if _, err = tx.Exec(ctx, `UPDATE bp_profiles SET xp = $2, updated_at = now() WHERE user_id = $1`, userID, newXP); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `
DELETE FROM bp_claims WHERE user_id = $1 AND season_id = $2 AND level > $3`, userID, season.ID, level); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `
DELETE FROM bp_case_tokens
WHERE user_id = $1 AND season_id = $2 AND source = 'level' AND source_level > $3 AND opened_at IS NULL`,
		userID, season.ID, level); err != nil {
		return nil, err
	}
	s, err := bpAdminStateTx(ctx, tx, userID, season)
	if err != nil {
		return nil, err
	}
	return s, tx.Commit(ctx)
}
