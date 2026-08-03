package referral_sql

type Referral struct {
	UserID              int
	ReferralCode        string
	RefXp               int
	TotalReferredVolume float64
	TotalEarned         float64
}