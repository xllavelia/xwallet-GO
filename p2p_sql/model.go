package p2p_sql

import (
	"errors"
	"time"
)

var ErrNotMerchant = errors.New("you must become a merchant to create listings")
var ErrInvalidListing = errors.New("invalid listing parameters")
var ErrInsufficientDeposit = errors.New("insufficient balance for merchant deposit")
var ErrListingNotFound = errors.New("listing not found")
var ErrListingInsufficientVolume = errors.New("not enough volume left in this listing")
var ErrAmountOutOfRange = errors.New("amount is outside the listing's limits")
var ErrDealNotFound = errors.New("deal not found")
var ErrDealNotPending = errors.New("this deal cannot be modified")
var ErrDealExpired = errors.New("this deal has expired")
var ErrCannotDealOwnListing = errors.New("cannot start a deal on your own listing")
var ErrAlreadyMerchant = errors.New("already a merchant")
var ErrHasActiveListings = errors.New("close all active listings before withdrawing your deposit")

var DepositAmountUsd = 50.0 // edit here to change the merchant guarantee deposit
var DealExpiryMinutes = 15

type Listing struct {
	ID                  int
	UserID              int
	Side                string
	BaseAsset           string
	AssetClass          string
	RateUsd             float64
	MinAmountUsd        float64
	MaxAmountUsd        float64
	RemainingBaseAmount float64
	PaymentNote         string
	Status              string
	CreatedAt           time.Time
}

type Deal struct {
	ID             int
	ListingID      int
	PosterUserID   int
	TakerUserID    int
	Side           string
	BaseAsset      string
	AssetClass     string
	BaseAmount     float64
	QuoteAmountUsd float64
	RateUsd        float64
	CommissionUsd  float64
	Status         string
	ExpiresAt      time.Time
	CompletedAt    *time.Time
	CreatedAt      time.Time
}

type Merchant struct {
	UserID         int
	IsVerified     bool
	DepositAmount  float64
	TotalDeals     int
	CompletedDeals int
	CreatedAt      time.Time
}
