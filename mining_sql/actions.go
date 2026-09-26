package mining_sql

import (
	"context"
	"errors"
	"time"

	"xwallet-server/wallet_sql"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Тонкая обёртка над wallet_sql, чтобы легко заменить в одном месте при необходимости.
func AdjustWalletBalanceTx(ctx context.Context, tx pgx.Tx, userID int, delta float64) error {
	return wallet_sql.AdjustBalanceTx(ctx, tx, userID, delta)
}

var (
	ErrUnknownServer     = errors.New("unknown server catalog id")
	ErrSlotLimitReached  = errors.New("server slot limit reached")
	ErrServerNotFound    = errors.New("server not found")
	ErrServerExhausted   = errors.New("server is exhausted")
	ErrAlreadyAlwaysOn   = errors.New("server never sleeps")
	ErrMaxRank           = errors.New("server already at max rank")
	ErrUnknownItem       = errors.New("unknown shop item")
	ErrUnknownDuration   = errors.New("unknown duration key")
	ErrInsufficientFunds = wallet_sql.ErrInsufficientBalanceTx
)

// ===================== ПОКУПКА СЕРВЕРА =====================
func BuyServer(ctx context.Context, pool *pgxpool.Pool, userID int, catalogID string) error {
	def, ok := GetServerDef(catalogID)
	if !ok {
		return ErrUnknownServer
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	profile, err := getOrCreateProfile(ctx, tx, userID)
	if err != nil {
		return err
	}

	var totalOwned int
	err = tx.QueryRow(ctx, `SELECT count(*) FROM mining_servers WHERE user_id = $1;`, userID).Scan(&totalOwned)
	if err != nil {
		return err
	}
	maxTotal := BaseMaxCopiesPerServer + profile.ExtraSlots
	if totalOwned >= maxTotal {
		return ErrSlotLimitReached
	}

	if err := AdjustWalletBalanceTx(ctx, tx, userID, -def.Price); err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO mining_servers (user_id, catalog_id, rank, purchased_at, last_tick_at)
		VALUES ($1, $2, 1, now(), now());
	`, userID, catalogID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `UPDATE mining_profile SET lifetime_spent = lifetime_spent + $1 WHERE user_id = $2;`, def.Price, userID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ===================== РАЗБУДИТЬ СЕРВЕР =====================

func WakeServer(ctx context.Context, pool *pgxpool.Pool, userID int, serverID int64) error {
	if err := Tick(ctx, pool, userID); err != nil {
		return err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var catalogID string
	var rank int
	var exhausted bool
	err = tx.QueryRow(ctx, `
		SELECT catalog_id, rank, exhausted FROM mining_servers WHERE id = $1 AND user_id = $2 FOR UPDATE;
	`, serverID, userID).Scan(&catalogID, &rank, &exhausted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrServerNotFound
		}
		return err
	}
	if exhausted {
		return ErrServerExhausted
	}

	def, ok := GetServerDef(catalogID)
	if !ok {
		return ErrUnknownServer
	}
	if def.SleepMinutes == 0 {
		return ErrAlreadyAlwaysOn
	}
	if rank == 10 && def.PermanentNoSleepAtMaxRank {
		return ErrAlreadyAlwaysOn
	}

	awakeUntil := time.Now().Add(time.Duration(def.SleepMinutes) * time.Minute)
	_, err = tx.Exec(ctx, `UPDATE mining_servers SET awake_until = $1, last_tick_at = now() WHERE id = $2;`, awakeUntil, serverID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ===================== УДАЛИТЬ СЕРВЕР =====================

// Возврат при удалении сервера, в долях от исходной цены (0.5 = 50%).
const ServerRefundRatio = 0.5

func DeleteServer(ctx context.Context, pool *pgxpool.Pool, userID int, serverID int64) error {
	if err := Tick(ctx, pool, userID); err != nil {
		return err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var catalogID string
	err = tx.QueryRow(ctx, `SELECT catalog_id FROM mining_servers WHERE id = $1 AND user_id = $2 FOR UPDATE;`, serverID, userID).Scan(&catalogID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrServerNotFound
		}
		return err
	}

	_, err = tx.Exec(ctx, `DELETE FROM mining_servers WHERE id = $1 AND user_id = $2;`, serverID, userID)
	if err != nil {
		return err
	}

	if def, ok := GetServerDef(catalogID); ok {
		refund := def.Price * ServerRefundRatio
		if err := AdjustWalletBalanceTx(ctx, tx, userID, refund); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// ===================== ПРОКАЧКА РАНГА =====================

func UpgradeServer(ctx context.Context, pool *pgxpool.Pool, userID int, serverID int64) error {
	if err := Tick(ctx, pool, userID); err != nil {
		return err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var catalogID string
	var rank int
	err = tx.QueryRow(ctx, `SELECT catalog_id, rank FROM mining_servers WHERE id = $1 AND user_id = $2 FOR UPDATE;`, serverID, userID).Scan(&catalogID, &rank)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrServerNotFound
		}
		return err
	}
	if rank >= 10 {
		return ErrMaxRank
	}

	def, ok := GetServerDef(catalogID)
	if !ok {
		return ErrUnknownServer
	}

	cost := def.Price * RankTiers[rank].UpgradeCostMult // RankTiers[rank] = тир целевого ранга rank+1
	if err := AdjustWalletBalanceTx(ctx, tx, userID, -cost); err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `UPDATE mining_servers SET rank = rank + 1 WHERE id = $1;`, serverID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE mining_profile SET lifetime_spent = lifetime_spent + $1 WHERE user_id = $2;`, cost, userID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ===================== ПОКУПКА ПРЕДМЕТОВ =====================

// BuyBuffItem — покупка одного из time-баффов (no_sleep / profit_boost / power_boost / power_boost_eff).
func BuyBuffItem(ctx context.Context, pool *pgxpool.Pool, userID int, buffType string, durationKey string) error {
	durations, ok := BuffCatalog[buffType]
	if !ok {
		return ErrUnknownItem
	}
	var dur BuffDuration
	found := false
	for _, d := range durations {
		if d.Key == durationKey {
			dur = d
			found = true
			break
		}
	}
	if !found {
		return ErrUnknownDuration
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := AdjustWalletBalanceTx(ctx, tx, userID, -dur.Price); err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO mining_buffs (user_id, buff_type, expires_at)
		VALUES ($1, $2, now() + ($3 * interval '1 hour'))
		ON CONFLICT (user_id, buff_type) DO UPDATE
		SET expires_at = GREATEST(mining_buffs.expires_at, now()) + ($3 * interval '1 hour');
	`, userID, buffType, dur.Hours)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO mining_profile (user_id, energy, max_energy, lifetime_spent, last_tick_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (user_id) DO UPDATE SET lifetime_spent = mining_profile.lifetime_spent + $4;
	`, userID, DefaultStartEnergy, DefaultMaxEnergy, dur.Price)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// BuyEnergyPack — разовая покупка энергии по одному из трёх пакетов.
func BuyEnergyPack(ctx context.Context, pool *pgxpool.Pool, userID int, packID string) error {
	pack, ok := GetEnergyPack(packID)
	if !ok {
		return ErrUnknownItem
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := getOrCreateProfile(ctx, tx, userID); err != nil {
		return err
	}
	if err := AdjustWalletBalanceTx(ctx, tx, userID, -pack.Price); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE mining_profile
		SET energy = LEAST(max_energy, energy + $1), lifetime_spent = lifetime_spent + $2
		WHERE user_id = $3;
	`, pack.Amount, pack.Price, userID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// BuyMaxEnergyExpander — навсегда увеличивает max_energy.
func BuyMaxEnergyExpander(ctx context.Context, pool *pgxpool.Pool, userID int) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := getOrCreateProfile(ctx, tx, userID); err != nil {
		return err
	}
	if err := AdjustWalletBalanceTx(ctx, tx, userID, -MaxEnergyExpanderPrice); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE mining_profile
		SET max_energy = max_energy + $1, lifetime_spent = lifetime_spent + $2
		WHERE user_id = $3;
	`, MaxEnergyExpanderAddAmount, MaxEnergyExpanderPrice, userID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// BuySlotExpander — навсегда увеличивает лимит копий на КАЖДЫЙ тип сервера.
func BuySlotExpander(ctx context.Context, pool *pgxpool.Pool, userID int) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := getOrCreateProfile(ctx, tx, userID); err != nil {
		return err
	}
	if err := AdjustWalletBalanceTx(ctx, tx, userID, -SlotExpanderPrice); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE mining_profile
		SET extra_slots = extra_slots + $1, lifetime_spent = lifetime_spent + $2
		WHERE user_id = $3;
	`, SlotExpanderAddSlots, SlotExpanderPrice, userID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}
