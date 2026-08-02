package card_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func GetCardByPlayerID(ctx context.Context, pool *pgxpool.Pool, playerID string) (CryptoCard, error) {
	sqlQuery := `
	SELECT cc.id, cc.card_number, cc.user_id, cc.btc_amount, cc.eth_amount, cc.sol_amount, cc.ton_amount, cc.valid_thru, cc.created_at
	FROM crypto_cards cc
	JOIN users u ON u.id = cc.user_id
	WHERE u.player_id = $1;
	`
	var c CryptoCard
	err := pool.QueryRow(ctx, sqlQuery, playerID).Scan(
		&c.ID, &c.CardNumber, &c.UserID, &c.BtcAmount, &c.EthAmount, &c.SolAmount, &c.TonAmount, &c.ValidThru, &c.CreatedAt,
	)

	return c, err
}
