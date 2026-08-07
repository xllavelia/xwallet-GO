package battlepass_sql

type RewardComponent struct {
	Kind  string  `json:"kind"`
	Value float64 `json:"value"`
	Days  int     `json:"days"`
	Label string  `json:"label"`
}

type Level struct {
	Level      int               `json:"level"`
	Components []RewardComponent `json:"components"`
}

var XPPerLevel = 1000

func rc(kind string, value float64, days int, label string) RewardComponent {
	return RewardComponent{Kind: kind, Value: value, Days: days, Label: label}
}

var ProCatalog = []Level{
	{1, []RewardComponent{rc("usdt", 5, 0, "")}},
	{2, []RewardComponent{rc("ref_xp", 10, 0, "")}},
	{3, []RewardComponent{rc("voucher_fee", 25, 2, "")}},
	{4, []RewardComponent{rc("lavx", 5, 0, "")}},
	{5, []RewardComponent{rc("case", 1, 0, "classico")}},
	{6, []RewardComponent{rc("usdt", 10, 0, "")}},
	{7, []RewardComponent{rc("ref_xp", 15, 0, "")}},
	{8, []RewardComponent{rc("voucher_fee", 50, 5, "")}},
	{9, []RewardComponent{rc("lavx", 5, 0, "")}},
	{10, []RewardComponent{
		rc("usdt", 20, 0, ""),
		rc("case", 1, 0, "classico"),
		rc("fee_boost", 3, 10, ""),
		rc("status", 0, 0, "force_pro"),
	}},
}

var PrimeCatalog = []Level{
	{1, []RewardComponent{rc("case", 1, 0, "classico")}},
	{2, []RewardComponent{rc("usdt", 10, 0, "")}},
	{3, []RewardComponent{rc("ref_xp", 20, 0, "")}},
	{4, []RewardComponent{rc("voucher_fee", 50, 5, "")}},
	{5, []RewardComponent{rc("lavx", 5, 0, "")}},
	{6, []RewardComponent{rc("xp_boost", 10, 10, "")}},
	{7, []RewardComponent{rc("case", 1, 0, "classico")}},
	{8, []RewardComponent{rc("usdt", 20, 0, "")}},
	{9, []RewardComponent{rc("ref_xp", 30, 0, "")}},
	{10, []RewardComponent{rc("voucher_fee", 100, 4, "")}},
	{11, []RewardComponent{rc("xp_boost", 20, 10, "")}},
	{12, []RewardComponent{rc("lavx", 10, 0, "")}},
	{13, []RewardComponent{rc("usdt", 30, 0, "")}},
	{14, []RewardComponent{
		rc("case", 1, 0, "elysium"),
		rc("fee_boost", 5, 10, ""),
		rc("lavx", 5, 0, ""),
		rc("ref_xp", 20, 0, ""),
		rc("status", 0, 0, "force_prime"),
	}},
}

var StarCatalog = []Level{
	{1, []RewardComponent{rc("usdt", 15, 0, "")}},
	{2, []RewardComponent{rc("ref_xp", 30, 0, "")}},
	{3, []RewardComponent{rc("lavx", 10, 0, "")}},
	{4, []RewardComponent{rc("xp_boost", 20, 5, "")}},
	{5, []RewardComponent{rc("case", 1, 0, "elysium")}},
	{6, []RewardComponent{rc("usdt", 25, 0, "")}},
	{7, []RewardComponent{rc("ref_xp", 30, 0, "")}},
	{8, []RewardComponent{rc("lavx", 10, 0, "")}},
	{9, []RewardComponent{rc("xp_boost", 30, 5, "")}},
	{10, []RewardComponent{rc("usdt", 30, 0, "")}},
	{11, []RewardComponent{rc("case", 1, 0, "elysium")}},
	{12, []RewardComponent{rc("ref_xp", 35, 0, "")}},
	{13, []RewardComponent{rc("lavx", 15, 0, "")}},
	{14, []RewardComponent{rc("xp_boost", 40, 5, "")}},
	{15, []RewardComponent{
		rc("case", 1, 0, "legendary"),
		rc("case", 1, 0, "elysium"),
		rc("status", 0, 0, "force_star"),
		rc("status", 0, 0, "legendary"),
	}},
}

func CatalogForTrack(track string) []Level {
	switch track {
	case "pro":
		return ProCatalog
	case "prime":
		return PrimeCatalog
	case "star":
		return StarCatalog
	}
	return nil
}
