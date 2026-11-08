package profile_sql

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func relationshipState(viewer, target int, requester *int, state *string) string {
	if viewer == target {
		return "self"
	}
	if requester == nil || state == nil {
		return "none"
	}
	if *state == "friend" {
		return "friend"
	}
	if *state != "pending" {
		return "none"
	}
	if *requester == viewer {
		return "outgoing"
	}
	return "incoming"
}

// Встречная заявка означает взаимное согласие; автором остаётся первый заявитель.
func nextFriendState(current, action string) (string, error) {
	switch action {
	case "request":
		switch current {
		case "none", "outgoing":
			return "outgoing", nil
		case "incoming", "friend":
			return "friend", nil
		}
	case "accept":
		if current == "incoming" || current == "friend" {
			return "friend", nil
		}
	case "decline":
		if current == "incoming" {
			return "none", nil
		}
	case "cancel":
		if current == "outgoing" {
			return "none", nil
		}
	case "remove":
		if current == "friend" {
			return "none", nil
		}
	default:
		return "", ErrInvalid
	}
	return "", ErrTransition
}

func validFriendAction(action string) bool {
	switch action {
	case "request", "accept", "decline", "cancel", "remove":
		return true
	}
	return false
}

func (s *Store) Friend(ctx context.Context, viewer, target, action string) (Relationship, error) {
	if ValidatePlayerID(target) != nil || !validFriendAction(action) {
		return Relationship{}, ErrInvalid
	}
	if target == viewer {
		return Relationship{}, ErrTransition
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Relationship{}, err
	}
	defer tx.Rollback(ctx)
	// Единый порядок блокировок защищает от гонок и взаимоблокировок даже без строки дружбы.
	rows, err := tx.Query(ctx, `SELECT u.id,u.player_id,u.is_admin,`+bannedSQL+`
FROM users u WHERE u.player_id IN ($1,$2) ORDER BY u.id FOR NO KEY UPDATE OF u`, viewer, target)
	if err != nil {
		return Relationship{}, err
	}
	accounts := map[string]account{}
	bans := map[string]bool{}
	for rows.Next() {
		var a account
		var banned bool
		if err := rows.Scan(&a.id, &a.player, &a.isAdmin, &banned); err != nil {
			rows.Close()
			return Relationship{}, err
		}
		accounts[a.player], bans[a.player] = a, banned
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Relationship{}, err
	}
	a, ok := accounts[viewer]
	if !ok {
		return Relationship{}, ErrUnauthorized
	}
	if bans[viewer] {
		return Relationship{}, ErrForbidden
	}
	b, ok := accounts[target]
	if !ok || bans[target] {
		return Relationship{}, ErrNotFound
	}
	low, high := a.id, b.id
	if low > high {
		low, high = high, low
	}
	var requester int
	var state string
	err = tx.QueryRow(ctx, `SELECT requester_id,state FROM community_friendships
WHERE user_low=$1 AND user_high=$2 FOR UPDATE`, low, high).Scan(&requester, &state)
	current := "none"
	if err == nil {
		current = relationshipState(a.id, b.id, &requester, &state)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Relationship{}, err
	}
	next, err := nextFriendState(current, action)
	if err != nil {
		return Relationship{}, err
	}
	if next != current {
		switch next {
		case "none":
			_, err = tx.Exec(ctx, `DELETE FROM community_friendships WHERE user_low=$1 AND user_high=$2`, low, high)
		case "outgoing":
			_, err = tx.Exec(ctx, `INSERT INTO community_friendships(user_low,user_high,requester_id,state)
VALUES ($1,$2,$3,'pending')`, low, high, a.id)
		case "friend":
			_, err = tx.Exec(ctx, `UPDATE community_friendships SET state='friend',updated_at=now()
WHERE user_low=$1 AND user_high=$2`, low, high)
		}
		if err != nil {
			return Relationship{}, err
		}
	}
	return Relationship{FriendState: next}, tx.Commit(ctx)
}

const newsListSQL = `SELECT id,title,summary,body,category,art,pinned,published_at
FROM community_news WHERE is_published AND published_at<=now()
ORDER BY pinned DESC,published_at DESC,id LIMIT 30`

func listNews(ctx context.Context, q queryer) ([]News, error) {
	rows, err := q.Query(ctx, newsListSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []News{}
	for rows.Next() {
		var n News
		if err := rows.Scan(&n.ID, &n.Title, &n.Summary, &n.Body, &n.Category, &n.Art, &n.Pinned, &n.PublishedAt); err != nil {
			return nil, err
		}
		if ValidNews(n) {
			result = append(result, n)
		}
	}
	return result, rows.Err()
}

const rankingCTE = `WITH ranked AS (
SELECT u.id, row_number() OVER (ORDER BY w.balance DESC,u.player_id ASC) AS rank
FROM users u JOIN wallets w ON w.user_id=u.id
LEFT JOIN user_profiles p ON p.user_id=u.id
WHERE NOT u.is_admin AND NOT ` + bannedSQL + ` AND COALESCE(p.show_balance,true)
) `

func (s *Store) Community(ctx context.Context, player string) (CommunityState, error) {
	result := CommunityState{News: []News{}, Friends: []PublicProfile{}, Incoming: []PublicProfile{},
		Outgoing: []PublicProfile{}, Leaderboard: []LeaderboardEntry{}}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	a, err := readAccount(ctx, tx, player, false, true)
	if err != nil {
		return result, err
	}
	result.News, err = listNews(ctx, tx)
	if err != nil {
		return result, err
	}
	rows, err := tx.Query(ctx, `SELECT `+publicColumns(true)+publicJoins+`
WHERE u.id<>$1 AND f.state IN ('pending','friend') AND NOT `+bannedSQL+`
ORDER BY lower(u.username),u.player_id`, a.id)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		p, err := scanPublic(rows, a.id, nil)
		if err != nil {
			rows.Close()
			return result, err
		}
		switch p.FriendState {
		case "friend":
			result.Friends = append(result.Friends, p)
		case "incoming":
			result.Incoming = append(result.Incoming, p)
		case "outgoing":
			result.Outgoing = append(result.Outgoing, p)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return result, err
	}
	rows, err = tx.Query(ctx, rankingCTE+`SELECT `+publicColumns(true)+`,r.rank`+publicJoins+`
JOIN ranked r ON r.id=u.id WHERE r.rank<=50 ORDER BY r.rank`, a.id)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var rank int
		p, err := scanPublic(rows, a.id, &rank)
		if err != nil {
			rows.Close()
			return result, err
		}
		result.Leaderboard = append(result.Leaderboard, LeaderboardEntry{PublicProfile: p, Rank: rank})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return result, err
	}
	var rank int
	err = tx.QueryRow(ctx, rankingCTE+`SELECT rank FROM ranked WHERE id=$1`, a.id).Scan(&rank)
	if err == nil {
		result.MyRank = &rank
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	return result, tx.Commit(ctx)
}

func (s *Store) Search(ctx context.Context, viewer, query string) ([]PublicProfile, error) {
	if err := ValidateSearch(query); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	a, err := readAccount(ctx, tx, viewer, false, true)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT `+publicColumns(true)+publicJoins+`
WHERE u.id<>$1 AND NOT `+bannedSQL+`
AND (u.player_id ILIKE '%' || $2 || '%' ESCAPE E'\\' OR u.username ILIKE '%' || $2 || '%' ESCAPE E'\\')
ORDER BY lower(u.username),u.player_id LIMIT 20`, a.id, escapeSearch(query))
	if err != nil {
		return nil, err
	}
	result := []PublicProfile{}
	for rows.Next() {
		p, err := scanPublic(rows, a.id, nil)
		if err != nil {
			rows.Close()
			return nil, err
		}
		result = append(result, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, tx.Commit(ctx)
}
