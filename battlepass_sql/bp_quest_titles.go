package battlepass_sql

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// BPRetranslateQuestTitles переписывает заголовки уже выданных квестов,
// чтобы смена текста в конфиге применялась без сброса прогресса.
func BPRetranslateQuestTitles(ctx context.Context, pool *pgxpool.Pool) error {
	templates := map[string]BPQuestTemplate{}
	for _, list := range [][]BPQuestTemplate{
		BPDailyQuests, BPDailyPaidQuests,
		BPWeeklyQuests, BPWeeklyPaidQuests,
		BPMonthlyQuests, BPMonthlyPaidQuests,
	} {
		for _, t := range list {
			templates[t.ID] = t
		}
	}

	rows, err := pool.Query(ctx, `SELECT id, template_id, target FROM bp_quests;`)
	if err != nil {
		return err
	}
	type questRow struct {
		id     int64
		tmplID string
		target float64
	}
	var quests []questRow
	for rows.Next() {
		var q questRow
		if err := rows.Scan(&q.id, &q.tmplID, &q.target); err != nil {
			rows.Close()
			return err
		}
		quests = append(quests, q)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, q := range quests {
		t, ok := templates[q.tmplID]
		if !ok {
			continue
		}
		title := fmt.Sprintf(t.Title, bpFormatTarget(t, q.target))
		if _, err := pool.Exec(ctx,
			`UPDATE bp_quests SET title = $1 WHERE id = $2 AND title <> $1;`, title, q.id); err != nil {
			return err
		}
	}
	return nil
}
