package battlepass_sql

import (
	"fmt"
	"time"
)

type BPQuestTemplate struct {
	ID     string
	Title  string
	Metric string
	Mode   string
	Min    float64
	Max    float64
	Step   float64
	Unit   string
}

type BPPeriodInfo struct {
	Key   string
	Start time.Time
	End   time.Time
}

func BPQuestXP(category string, paid bool) int {
	switch category {
	case "daily":
		if paid {
			return 180
		}
		return 140
	case "weekly":
		if paid {
			return 600
		}
		return 450
	case "monthly":
		if paid {
			return 1200
		}
		return 900
	}
	return 100
}

func BPQuestCounts(category string) (normal int, paid int) {
	switch category {
	case "daily":
		return 4, 2
	case "weekly":
		return 3, 1
	case "monthly":
		return 3, 1
	}
	return 3, 0
}

func BPQuestCategoryLabel(category string) string {
	switch category {
	case "daily":
		return "Daily"
	case "weekly":
		return "Weekly"
	case "monthly":
		return "Monthly"
	}
	return category
}

func bpWeekStart(t time.Time) time.Time {
	t = t.UTC()
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return day.AddDate(0, 0, 1-wd)
}

func BPPeriod(category string, now time.Time) BPPeriodInfo {
	now = now.UTC()
	switch category {
	case "daily":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return BPPeriodInfo{Key: "D:" + start.Format("2006-01-02"), Start: start, End: start.AddDate(0, 0, 1)}
	case "weekly":
		start := bpWeekStart(now)
		year, week := start.ISOWeek()
		return BPPeriodInfo{Key: fmt.Sprintf("W:%04d-W%02d", year, week), Start: start, End: start.AddDate(0, 0, 7)}
	case "monthly":
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return BPPeriodInfo{Key: "M:" + start.Format("2006-01"), Start: start, End: start.AddDate(0, 1, 0)}
	}
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return BPPeriodInfo{Key: "D:" + start.Format("2006-01-02"), Start: start, End: start.AddDate(0, 0, 1)}
}

func BPPeriodKey(category string, now time.Time) string {
	return BPPeriod(category, now).Key
}

func BPCurrentPeriodKeys(now time.Time) []string {
	return []string{
		BPPeriodKey("daily", now),
		BPPeriodKey("weekly", now),
		BPPeriodKey("monthly", now),
	}
}

func BPQuestPool(category string, paid bool) []BPQuestTemplate {
	switch category {
	case "daily":
		if paid {
			return BPDailyPaidQuests
		}
		return BPDailyQuests
	case "weekly":
		if paid {
			return BPWeeklyPaidQuests
		}
		return BPWeeklyQuests
	case "monthly":
		if paid {
			return BPMonthlyPaidQuests
		}
		return BPMonthlyQuests
	}
	return nil
}

var BPDailyQuests = []BPQuestTemplate{
	{ID: "pixel_rounds", Title: "Play %s Pixel rounds", Metric: "pixel_rounds", Mode: "count", Min: 5, Max: 10, Step: 1, Unit: "count"},
	{ID: "flip_rounds", Title: "Play %s Coin Flip rounds", Metric: "flip_rounds", Mode: "count", Min: 5, Max: 10, Step: 1, Unit: "count"},
	{ID: "rocket_rounds", Title: "Join %s Rocket rounds", Metric: "rocket_rounds", Mode: "count", Min: 3, Max: 6, Step: 1, Unit: "count"},
	{ID: "margin_open", Title: "Open %s margin trades", Metric: "margin_open_usd", Mode: "count", Min: 1, Max: 3, Step: 1, Unit: "count"},
	{ID: "timed_open", Title: "Open %s timed trades", Metric: "timed_open_usd", Mode: "count", Min: 1, Max: 3, Step: 1, Unit: "count"},
	{ID: "margin_volume", Title: "Trade %s volume on margin", Metric: "margin_volume_usd", Mode: "sum", Min: 100, Max: 250, Step: 25, Unit: "usd"},
	{ID: "timed_volume", Title: "Trade %s volume in timed trades", Metric: "timed_volume_usd", Mode: "sum", Min: 100, Max: 250, Step: 25, Unit: "usd"},
	{ID: "tickets_daily", Title: "Open %s tickets", Metric: "tickets_opened", Mode: "count", Min: 2, Max: 5, Step: 1, Unit: "count"},
	{ID: "slots_profit_daily", Title: "Win %s profit in Slots", Metric: "slots_profit_usd", Mode: "sum", Min: 5, Max: 25, Step: 5, Unit: "usd"},
	{ID: "transfer_daily", Title: "Send %s in transfers", Metric: "transfer_sent_usd", Mode: "sum", Min: 20, Max: 100, Step: 20, Unit: "usd"},
	{ID: "mining_income_daily", Title: "Earn %s from Mining", Metric: "mining_income_usd", Mode: "sum", Min: 5, Max: 20, Step: 5, Unit: "usd"},
	{ID: "empire_income_daily", Title: "Earn %s from Empire", Metric: "empire_income_usd", Mode: "sum", Min: 5, Max: 20, Step: 5, Unit: "usd"},
	{ID: "market_volume_daily", Title: "Trade %s on the Market", Metric: "market_volume_usd", Mode: "sum", Min: 20, Max: 100, Step: 20, Unit: "usd"},
	{ID: "slots_spins_daily", Title: "Spin Slots %s times", Metric: "slots_spins", Mode: "count", Min: 20, Max: 60, Step: 10, Unit: "count"},
	{ID: "pixel_safe_daily", Title: "Open %s safe Pixel cells", Metric: "pixel_safe_cells", Mode: "count", Min: 3, Max: 8, Step: 1, Unit: "count"},
	{ID: "savings_deposit", Title: "Deposit %s into Savings", Metric: "savings_deposit_usd", Mode: "sum", Min: 25, Max: 100, Step: 25, Unit: "usd"},
	{ID: "daily_rewards_daily", Title: "Claim %s daily rewards", Metric: "daily_rewards_claimed", Mode: "count", Min: 1, Max: 1, Step: 1, Unit: "count"},
	{ID: "empire_upgrades_daily", Title: "Upgrade Empire objects %s times", Metric: "empire_upgrades", Mode: "count", Min: 1, Max: 3, Step: 1, Unit: "count"},
	{ID: "mining_upgrades_daily", Title: "Upgrade Mining servers %s times", Metric: "mining_upgrades", Mode: "count", Min: 1, Max: 2, Step: 1, Unit: "count"},
}

var BPDailyPaidQuests = []BPQuestTemplate{
	{ID: "margin_volume_paid", Title: "Trade %s volume on margin", Metric: "margin_volume_usd", Mode: "sum", Min: 300, Max: 500, Step: 50, Unit: "usd"},
	{ID: "slots_profit_paid", Title: "Win %s profit in Slots", Metric: "slots_profit_usd", Mode: "sum", Min: 40, Max: 80, Step: 10, Unit: "usd"},
	{ID: "rocket_cashout_paid", Title: "Cash out at %s in Rocket", Metric: "rocket_cashout_x", Mode: "max", Min: 2, Max: 4, Step: 1, Unit: "x"},
	{ID: "tickets_paid", Title: "Open %s tickets", Metric: "tickets_opened", Mode: "count", Min: 10, Max: 15, Step: 1, Unit: "count"},
	{ID: "market_volume_paid", Title: "Trade %s on the Market", Metric: "market_volume_usd", Mode: "sum", Min: 150, Max: 300, Step: 50, Unit: "usd"},
	{ID: "pixel_safe_paid", Title: "Open %s safe Pixel cells", Metric: "pixel_safe_cells", Mode: "count", Min: 12, Max: 20, Step: 2, Unit: "count"},
}

var BPWeeklyQuests = []BPQuestTemplate{
	{ID: "pixel_safe_weekly", Title: "Open %s safe Pixel cells", Metric: "pixel_safe_cells", Mode: "count", Min: 20, Max: 30, Step: 5, Unit: "count"},
	{ID: "referrals_weekly", Title: "Invite %s friend", Metric: "referrals_joined", Mode: "count", Min: 1, Max: 1, Step: 1, Unit: "count"},
	{ID: "rocket_cashout_weekly", Title: "Cash out at %s in Rocket", Metric: "rocket_cashout_x", Mode: "max", Min: 3, Max: 5, Step: 1, Unit: "x"},
	{ID: "flip_streak_weekly", Title: "Hit a %s red/black streak in Coin Flip", Metric: "flip_color_streak", Mode: "max", Min: 3, Max: 4, Step: 1, Unit: "count"},
	{ID: "empire_build_weekly", Title: "Build %s Empire buildings", Metric: "empire_buildings_built", Mode: "count", Min: 1, Max: 2, Step: 1, Unit: "count"},
	{ID: "mining_servers_weekly", Title: "Buy %s Mining servers", Metric: "mining_servers_bought", Mode: "count", Min: 1, Max: 2, Step: 1, Unit: "count"},
	{ID: "daily_rewards_weekly", Title: "Claim daily rewards %s times", Metric: "daily_rewards_claimed", Mode: "count", Min: 5, Max: 7, Step: 1, Unit: "count"},
	{ID: "savings_interest_weekly", Title: "Earn %s interest in Savings", Metric: "savings_interest_usd", Mode: "sum", Min: 3, Max: 10, Step: 1, Unit: "usd"},
	{ID: "tickets_weekly", Title: "Open %s tickets", Metric: "tickets_opened", Mode: "count", Min: 30, Max: 60, Step: 10, Unit: "count"},
	{ID: "slots_spins_weekly", Title: "Spin Slots %s times", Metric: "slots_spins", Mode: "count", Min: 250, Max: 400, Step: 50, Unit: "count"},
	{ID: "trade_profit_weekly", Title: "Make %s trading profit", Metric: "trade_profit_usd", Mode: "sum", Min: 50, Max: 150, Step: 25, Unit: "usd"},
	{ID: "market_volume_weekly", Title: "Trade %s on the Market", Metric: "market_volume_usd", Mode: "sum", Min: 200, Max: 500, Step: 50, Unit: "usd"},
	{ID: "empire_income_weekly", Title: "Earn %s from Empire", Metric: "empire_income_usd", Mode: "sum", Min: 40, Max: 100, Step: 20, Unit: "usd"},
	{ID: "mining_income_weekly", Title: "Earn %s from Mining", Metric: "mining_income_usd", Mode: "sum", Min: 40, Max: 100, Step: 20, Unit: "usd"},
	{ID: "slots_profit_weekly", Title: "Win %s profit in Slots", Metric: "slots_profit_usd", Mode: "sum", Min: 50, Max: 150, Step: 25, Unit: "usd"},
}

var BPWeeklyPaidQuests = []BPQuestTemplate{
	{ID: "trade_profit_weekly_paid", Title: "Make %s trading profit", Metric: "trade_profit_usd", Mode: "sum", Min: 400, Max: 600, Step: 100, Unit: "usd"},
	{ID: "slots_diamonds_weekly", Title: "Land %s Diamonds in Slots", Metric: "slots_diamonds", Mode: "count", Min: 1, Max: 1, Step: 1, Unit: "count"},
	{ID: "pixel_safe_weekly_paid", Title: "Open %s safe Pixel cells", Metric: "pixel_safe_cells", Mode: "count", Min: 50, Max: 70, Step: 10, Unit: "count"},
	{ID: "rocket_cashout_weekly_paid", Title: "Cash out at %s in Rocket", Metric: "rocket_cashout_x", Mode: "max", Min: 5, Max: 7, Step: 1, Unit: "x"},
}

var BPMonthlyQuests = []BPQuestTemplate{
	{ID: "pixel_safe_monthly", Title: "Open %s safe Pixel cells", Metric: "pixel_safe_cells", Mode: "count", Min: 200, Max: 200, Step: 1, Unit: "count"},
	{ID: "slots_diamonds_monthly", Title: "Land %s Diamonds in Slots", Metric: "slots_diamonds", Mode: "count", Min: 3, Max: 3, Step: 1, Unit: "count"},
	{ID: "trade_profit_monthly", Title: "Make %s trading profit", Metric: "trade_profit_usd", Mode: "sum", Min: 500, Max: 500, Step: 1, Unit: "usd"},
	{ID: "mining_power_monthly", Title: "Earn %s Mining Power", Metric: "mining_power_gained", Mode: "sum", Min: 1000, Max: 1000, Step: 1, Unit: "power"},
	{ID: "margin_volume_monthly", Title: "Trade %s volume on margin", Metric: "margin_volume_usd", Mode: "sum", Min: 1500, Max: 1500, Step: 1, Unit: "usd"},
	{ID: "tickets_monthly", Title: "Open %s tickets", Metric: "tickets_opened", Mode: "count", Min: 500, Max: 500, Step: 1, Unit: "count"},
	{ID: "slots_spins_monthly", Title: "Spin Slots %s times", Metric: "slots_spins", Mode: "count", Min: 3000, Max: 3000, Step: 1, Unit: "count"},
	{ID: "empire_build_monthly", Title: "Build %s Empire buildings", Metric: "empire_buildings_built", Mode: "count", Min: 3, Max: 3, Step: 1, Unit: "count"},
	{ID: "mining_income_monthly", Title: "Earn %s from Mining", Metric: "mining_income_usd", Mode: "sum", Min: 300, Max: 300, Step: 1, Unit: "usd"},
	{ID: "empire_income_monthly", Title: "Earn %s from Empire", Metric: "empire_income_usd", Mode: "sum", Min: 300, Max: 300, Step: 1, Unit: "usd"},
}

var BPMonthlyPaidQuests = []BPQuestTemplate{
	{ID: "trade_profit_monthly_paid", Title: "Make %s trading profit", Metric: "trade_profit_usd", Mode: "sum", Min: 1500, Max: 1500, Step: 1, Unit: "usd"},
	{ID: "slots_diamonds_monthly_paid", Title: "Land %s Diamonds in Slots", Metric: "slots_diamonds", Mode: "count", Min: 5, Max: 5, Step: 1, Unit: "count"},
	{ID: "pixel_safe_monthly_paid", Title: "Open %s safe Pixel cells", Metric: "pixel_safe_cells", Mode: "count", Min: 400, Max: 400, Step: 1, Unit: "count"},
	{ID: "mining_power_monthly_paid", Title: "Earn %s Mining Power", Metric: "mining_power_gained", Mode: "sum", Min: 2000, Max: 2000, Step: 1, Unit: "power"},
	{ID: "empire_income_monthly_paid", Title: "Earn %s from Empire", Metric: "empire_income_usd", Mode: "sum", Min: 500, Max: 500, Step: 1, Unit: "usd"},
}
