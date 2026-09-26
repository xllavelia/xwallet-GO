package rewards_sql

import "time"

const (
	TrackDays = 30
	Cooldown  = 24 * time.Hour
)

type RewardComponent struct {
	Kind  string  `json:"kind"`
	Value float64 `json:"value"`
}

type DayReward struct {
	Day        int               `json:"day"`
	Components []RewardComponent `json:"components"`
}

func CatalogFor(track string) []DayReward {
	switch track {
	case "start":
		return startCatalog
	case "pro":
		return proCatalog
	}
	return nil
}

func usdt(v float64) RewardComponent    { return RewardComponent{Kind: "usdt", Value: v} }
func lavx(v float64) RewardComponent    { return RewardComponent{Kind: "lavx", Value: v} }
func voucher(v float64) RewardComponent { return RewardComponent{Kind: "voucher", Value: v} }
func refXp(v float64) RewardComponent   { return RewardComponent{Kind: "ref_xp", Value: v} }
func passXp(v float64) RewardComponent  { return RewardComponent{Kind: "pass_xp", Value: v} }

func day(n int, comps ...RewardComponent) DayReward {
	return DayReward{Day: n, Components: comps}
}

var startCatalog = []DayReward{
	day(1, usdt(10)),
	day(2, passXp(10)),
	day(3, usdt(3)),
	day(4, refXp(5)),
	day(5, voucher(1)),
	day(6, lavx(2)),
	day(7, usdt(3)),
	day(8, passXp(15)),
	day(9, usdt(3)),
	day(10, lavx(2)),
	day(11, refXp(10)),
	day(12, usdt(3)),
	day(13, passXp(15)),
	day(14, voucher(2)),
	day(15, usdt(10)),
	day(16, lavx(2)),
	day(17, refXp(10)),
	day(18, usdt(3)),
	day(19, passXp(20)),
	day(20, voucher(2)),
	day(21, usdt(3)),
	day(22, lavx(2)),
	day(23, refXp(15)),
	day(24, usdt(3)),
	day(25, passXp(25)),
	day(26, voucher(3)),
	day(27, usdt(3)),
	day(28, lavx(2)),
	day(29, refXp(20)),
	day(30, usdt(25), lavx(5)),
}

var proCatalog = []DayReward{
	day(1, usdt(20)),
	day(2, passXp(250)),
	day(3, usdt(5)),
	day(4, refXp(15)),
	day(5, voucher(3)),
	day(6, lavx(3)),
	day(7, usdt(5)),
	day(8, passXp(300)),
	day(9, usdt(1.50)),
	day(10, lavx(3)),
	day(11, refXp(20)),
	day(12, usdt(5)),
	day(13, passXp(300)),
	day(14, voucher(3)),
	day(15, usdt(5)),
	day(16, lavx(3)),
	day(17, refXp(25)),
	day(18, usdt(5)),
	day(19, passXp(400)),
	day(20, voucher(2)),
	day(21, usdt(5)),
	day(22, lavx(3)),
	day(23, refXp(25)),
	day(24, usdt(5)),
	day(25, passXp(500)),
	day(26, voucher(4)),
	day(27, usdt(5)),
	day(28, lavx(5)),
	day(29, refXp(40)),
	day(30, usdt(50), lavx(10), voucher(5), passXp(700)),
}
