package referral_sql

import "regexp"

var ReferralCodePattern = regexp.MustCompile(`^REF-[A-Z]{2}[0-9]{2}$`)
