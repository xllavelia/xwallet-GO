package referral_sql

var LevelNames = []string{"", "Scout", "Networker", "Connector", "Ambassador", "Elite Partner"}
var LevelRates = []float64{0, 1, 3, 5, 7, 10}

func LevelFromXP(xp int) int {
	level := 1 + xp/100
	if level > 5 {
		level = 5
	}
	return level
}

func RateForLevel(level int) float64 {
	if level < 1 {
		level = 1
	}
	if level > 5 {
		level = 5
	}
	return LevelRates[level]
}

func NameForLevel(level int) string {
	if level < 1 {
		level = 1
	}
	if level > 5 {
		level = 5
	}
	return LevelNames[level]
}
