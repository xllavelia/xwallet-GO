package pixel_sql

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"

	"xwallet-server/bankcards_sql"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func marshalInts(ints []int) []byte {
	if ints == nil {
		ints = []int{}
	}
	b, _ := json.Marshal(ints)
	return b
}
func unmarshalInts(b []byte) []int {
	var out []int
	json.Unmarshal(b, &out)
	if out == nil {
		out = []int{}
	}
	return out
}
func pickRandom(items []int, n int) []int {
	cp := make([]int, len(items))
	copy(cp, items)
	rand.Shuffle(len(cp), func(i, j int) { cp[i], cp[j] = cp[j], cp[i] })
	return cp[:n]
}
func generateMinePositions(totalCells int, mineCount int) []int {
	all := make([]int, totalCells)
	for i := range all {
		all[i] = i
	}
	rand.Shuffle(len(all), func(i, j int) { all[i], all[j] = all[j], all[i] })
	if mineCount > totalCells {
		mineCount = totalCells
	}
	mines := make([]int, mineCount)
	copy(mines, all[:mineCount])
	return mines
}

func isInSet(idx int, set []int) bool {
	for _, v := range set {
		if v == idx {
			return true
		}
	}
	return false
}

func countSafe(revealed []int, mines []int) int {
	count := 0
	for _, c := range revealed {
		if !isInSet(c, mines) {
			count++
		}
	}
	return count
}

func ensureStatsRow(ctx context.Context, tx pgx.Tx, userID int) error {
	_, err := tx.Exec(ctx, `INSERT INTO pixel_user_stats (user_id) VALUES ($1) ON CONFLICT (user_id) DO NOTHING;`, userID)
	return err
}

func updateStatsOnRevealTx(ctx context.Context, tx pgx.Tx, userID int, wasMine bool) error {
	if err := ensureStatsRow(ctx, tx, userID); err != nil {
		return err
	}
	if wasMine {
		_, err := tx.Exec(ctx, `UPDATE pixel_user_stats SET total_cells_opened = total_cells_opened + 1 WHERE user_id = $1;`, userID)
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE pixel_user_stats SET total_cells_opened = total_cells_opened + 1, safe_cells_opened = safe_cells_opened + 1 WHERE user_id = $1;`, userID)
	return err
}

func finalizeStatsTx(ctx context.Context, tx pgx.Tx, userID int, won bool, payout float64, stake float64) error {
	if err := ensureStatsRow(ctx, tx, userID); err != nil {
		return err
	}
	wonInt := 0
	if won {
		wonInt = 1
	}
	_, err := tx.Exec(ctx, `
		UPDATE pixel_user_stats SET total_games = total_games + 1, games_won = games_won + $1, total_profit = total_profit + $2
		WHERE user_id = $3;
	`, wonInt, payout-stake, userID)
	return err
}
func PlaceBet(ctx context.Context, pool *pgxpool.Pool, userID int, roundID int, amount float64) (int, error) {
	fundingSource, err := bankcards_sql.ResolveFundingSource(ctx, pool, userID)
	if err != nil {
		return 0, err
	}
	cols, rows := GridCols, GridRows
	mines := generateMinePositions(cols*rows, MineCount())
	preRevealed := pickRandom(mines, 2)
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if err := bankcards_sql.AdjustFundingBalance(ctx, tx, fundingSource, -amount); err != nil {
		if errors.Is(err, bankcards_sql.ErrInsufficientFunds) {
			return 0, ErrInsufficientFunds
		}
		return 0, err
	}
	var boardID int
	err = tx.QueryRow(ctx, `
	INSERT INTO pixel_boards (round_id, user_id, amount, grid_cols, grid_rows,
	mine_positions, revealed_cells, lives_remaining, status)
	VALUES ($1,$2,$3,$4,$5,$6,$7,2,'active') RETURNING id;
	`, roundID, userID, amount, cols, rows, marshalInts(mines), marshalInts(preRevealed)).Scan(&boardID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return 0, ErrAlreadyBet
		}
		return 0, err
	}
	return boardID, tx.Commit(ctx)
}

func getBoardForUpdate(ctx context.Context, tx pgx.Tx, roundID int, userID int) (Board, error) {
	var b Board
	var minesRaw, revealedRaw []byte
	err := tx.QueryRow(ctx, `
		SELECT id, round_id, user_id, amount, grid_cols, grid_rows, mine_positions, revealed_cells, lives_remaining, status, payout, created_at, ended_at
		FROM pixel_boards WHERE round_id = $1 AND user_id = $2 FOR UPDATE;
	`, roundID, userID).Scan(&b.ID, &b.RoundID, &b.UserID, &b.Amount, &b.GridCols, &b.GridRows, &minesRaw, &revealedRaw, &b.LivesRemaining, &b.Status, &b.Payout, &b.CreatedAt, &b.EndedAt)
	if err != nil {
		return Board{}, err
	}
	b.MinePositions = unmarshalInts(minesRaw)
	b.RevealedCells = unmarshalInts(revealedRaw)
	return b, nil
}

type RevealResult struct {
	IsMine            bool
	LivesRemaining    int
	SafeCellsCount    int
	CurrentMultiplier float64
	CurrentPayout     float64
	GameOver          bool
	MinePositions     []int
}

func RevealCell(ctx context.Context, pool *pgxpool.Pool, userID int, roundID int, cellIndex int) (RevealResult, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return RevealResult{}, err
	}
	defer tx.Rollback(ctx)

	board, err := getBoardForUpdate(ctx, tx, roundID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RevealResult{}, ErrNoActiveBoard
		}
		return RevealResult{}, err
	}
	if board.Status != "active" {
		return RevealResult{}, ErrNoActiveBoard
	}
	total := board.GridCols * board.GridRows
	if cellIndex < 0 || cellIndex >= total {
		return RevealResult{}, ErrInvalidCell
	}
	if isInSet(cellIndex, board.RevealedCells) {
		return RevealResult{}, ErrCellAlreadyRevealed
	}

	isMine := isInSet(cellIndex, board.MinePositions)
	newRevealed := append(board.RevealedCells, cellIndex)
	newLives := board.LivesRemaining
	if isMine {
		newLives--
	}

	safeCount := countSafe(newRevealed, board.MinePositions)
	multiplier := 1 + float64(safeCount)*(PercentPerCell/100)
	currentPayout := board.Amount * multiplier
	gameOver := newLives <= 0

	if gameOver {
		zero := 0.0
		_, err = tx.Exec(ctx, `UPDATE pixel_boards SET revealed_cells=$1, lives_remaining=$2, status='lost', payout=$3, ended_at=now() WHERE id=$4;`,
			marshalInts(newRevealed), newLives, zero, board.ID)
	} else {
		_, err = tx.Exec(ctx, `UPDATE pixel_boards SET revealed_cells=$1, lives_remaining=$2 WHERE id=$3;`,
			marshalInts(newRevealed), newLives, board.ID)
	}
	if err != nil {
		return RevealResult{}, err
	}
	if err := updateStatsOnRevealTx(ctx, tx, userID, isMine); err != nil {
		return RevealResult{}, err
	}
	if gameOver {
		if err := finalizeStatsTx(ctx, tx, userID, false, 0, board.Amount); err != nil {
			return RevealResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return RevealResult{}, err
	}

	result := RevealResult{IsMine: isMine, LivesRemaining: newLives, SafeCellsCount: safeCount, CurrentMultiplier: multiplier, CurrentPayout: currentPayout, GameOver: gameOver}
	if gameOver {
		result.MinePositions = board.MinePositions
	}
	return result, nil
}

func CashOut(ctx context.Context, pool *pgxpool.Pool, userID int, roundID int) (float64, []int, error) {
	fundingSource, err := bankcards_sql.ResolveFundingSource(ctx, pool, userID)
	if err != nil {
		return 0, nil, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback(ctx)

	board, err := getBoardForUpdate(ctx, tx, roundID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil, ErrNoActiveBoard
		}
		return 0, nil, err
	}
	if board.Status != "active" {
		return 0, nil, ErrNoActiveBoard
	}

	safeCount := countSafe(board.RevealedCells, board.MinePositions)
	multiplier := 1 + float64(safeCount)*(PercentPerCell/100)
	payout := board.Amount * multiplier

	if err := bankcards_sql.AdjustFundingBalance(ctx, tx, fundingSource, payout); err != nil {
		return 0, nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE pixel_boards SET status='cashed_out', payout=$1, ended_at=now() WHERE id=$2;`, payout, board.ID); err != nil {
		return 0, nil, err
	}
	if err := finalizeStatsTx(ctx, tx, userID, true, payout, board.Amount); err != nil {
		return 0, nil, err
	}
	return payout, nil, tx.Commit(ctx)
}

type BoardStateDTO struct {
	Exists            bool
	Status            string
	LivesRemaining    int
	RevealedCells     []int
	SafeCellsCount    int
	CurrentMultiplier float64
	CurrentPayout     float64
	Amount            float64
	MinePositions     []int
	Payout            *float64
}

func GetBoardState(ctx context.Context, pool *pgxpool.Pool, userID int, roundID int) (BoardStateDTO, error) {
	var b Board
	var minesRaw, revealedRaw []byte
	err := pool.QueryRow(ctx, `
		SELECT id, round_id, user_id, amount, grid_cols, grid_rows, mine_positions, revealed_cells, lives_remaining, status, payout, created_at, ended_at
		FROM pixel_boards WHERE round_id = $1 AND user_id = $2;
	`, roundID, userID).Scan(&b.ID, &b.RoundID, &b.UserID, &b.Amount, &b.GridCols, &b.GridRows, &minesRaw, &revealedRaw, &b.LivesRemaining, &b.Status, &b.Payout, &b.CreatedAt, &b.EndedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BoardStateDTO{Exists: false, RevealedCells: []int{}, MinePositions: []int{}}, nil
		}
		return BoardStateDTO{}, err
	}
	b.MinePositions = unmarshalInts(minesRaw)
	b.RevealedCells = unmarshalInts(revealedRaw)

	safeCount := countSafe(b.RevealedCells, b.MinePositions)
	multiplier := 1 + float64(safeCount)*(PercentPerCell/100)

	dto := BoardStateDTO{
		Exists: true, Status: b.Status, LivesRemaining: b.LivesRemaining,
		RevealedCells: b.RevealedCells, SafeCellsCount: safeCount,
		CurrentMultiplier: multiplier, CurrentPayout: b.Amount * multiplier,
		Amount: b.Amount, Payout: b.Payout, MinePositions: b.MinePositions,
	}
	if b.Status != "active" {
		dto.MinePositions = b.MinePositions
	}
	return dto, nil
}

func GetActiveBoardsForRound(ctx context.Context, pool *pgxpool.Pool, roundID int) ([]Board, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, round_id, user_id, amount, grid_cols, grid_rows, mine_positions, revealed_cells, lives_remaining, status, payout, created_at, ended_at
		FROM pixel_boards WHERE round_id = $1 AND status = 'active';
	`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Board
	for rows.Next() {
		var b Board
		var minesRaw, revealedRaw []byte
		if err := rows.Scan(&b.ID, &b.RoundID, &b.UserID, &b.Amount, &b.GridCols, &b.GridRows, &minesRaw, &revealedRaw, &b.LivesRemaining, &b.Status, &b.Payout, &b.CreatedAt, &b.EndedAt); err != nil {
			return nil, err
		}
		b.MinePositions = unmarshalInts(minesRaw)
		b.RevealedCells = unmarshalInts(revealedRaw)
		result = append(result, b)
	}
	return result, rows.Err()
}

func AutoCashOutBoard(ctx context.Context, pool *pgxpool.Pool, board Board) error {
	fundingSource, err := bankcards_sql.ResolveFundingSource(ctx, pool, board.UserID)
	if err != nil {
		return err
	}
	safeCount := countSafe(board.RevealedCells, board.MinePositions)
	multiplier := 1 + float64(safeCount)*(PercentPerCell/100)
	payout := board.Amount * multiplier

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `UPDATE pixel_boards SET status='expired', payout=$1, ended_at=now() WHERE id=$2 AND status='active';`, payout, board.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil // уже settled параллельным cashout/loss — гонка исключена
	}
	if err := bankcards_sql.AdjustFundingBalance(ctx, tx, fundingSource, payout); err != nil {
		return err
	}
	if err := finalizeStatsTx(ctx, tx, board.UserID, true, payout, board.Amount); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
