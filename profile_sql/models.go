package profile_sql

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalid          = errors.New("invalid profile request")
	ErrForbidden        = errors.New("account or cosmetic access denied")
	ErrNotFound         = errors.New("player not found")
	ErrUnauthorized     = errors.New("account no longer exists")
	ErrNotOwned         = errors.New("cosmetic is not owned")
	ErrNotPurchasable   = errors.New("cosmetic is not purchasable")
	ErrInsufficientLavx = errors.New("not enough LAVX")
	ErrWalletMissing    = errors.New("wallet not found")
	ErrUsernameTaken    = errors.New("username is already in use")
	ErrTransition       = errors.New("friend action is not allowed in this state")
)

type PublicProfile struct {
	PlayerID    string    `json:"playerId"`
	Username    string    `json:"username"`
	Bio         string    `json:"bio"`
	AvatarURL   string    `json:"avatarUrl"`
	ThemeID     string    `json:"themeId"`
	EmblemID    string    `json:"emblemId"`
	PatternID   string    `json:"patternId"`
	IsAdmin     bool      `json:"isAdmin"`
	CreatedAt   time.Time `json:"createdAt"`
	ShowBalance bool      `json:"showBalance"`
	Balance     *float64  `json:"balance,omitempty"`
	FriendState string    `json:"friendState"`
}

type OwnState struct {
	Profile     PublicProfile `json:"profile"`
	Catalog     []Cosmetic    `json:"catalog"`
	LavxBalance float64       `json:"lavxBalance"`
}

type UpdateInput struct {
	Username    string `json:"username"`
	Bio         string `json:"bio"`
	ShowBalance bool   `json:"showBalance"`
}

type EquipInput struct {
	ThemeID   string `json:"themeId"`
	EmblemID  string `json:"emblemId"`
	PatternID string `json:"patternId"`
}

type News struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	Body        string    `json:"body"`
	Category    string    `json:"category"`
	Art         string    `json:"art"`
	Pinned      bool      `json:"pinned"`
	PublishedAt time.Time `json:"publishedAt"`
}

type LeaderboardEntry struct {
	PublicProfile
	Rank int `json:"rank"`
}

type CommunityState struct {
	News        []News             `json:"news"`
	Friends     []PublicProfile    `json:"friends"`
	Incoming    []PublicProfile    `json:"incoming"`
	Outgoing    []PublicProfile    `json:"outgoing"`
	Leaderboard []LeaderboardEntry `json:"leaderboard"`
	MyRank      *int               `json:"myRank"`
}

type Relationship struct {
	FriendState string `json:"friendState"`
}

func validText(s string, min, max int) bool {
	if !utf8.ValidString(s) {
		return false
	}
	n := utf8.RuneCountInString(s)
	if n < min || n > max {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			return false
		}
	}
	return true
}

func ValidateUpdate(in UpdateInput) error {
	if in.Username != strings.TrimSpace(in.Username) || !validText(in.Username, 3, 32) || !validText(in.Bio, 0, 160) {
		return ErrInvalid
	}
	return nil
}

func ValidatePlayerID(id string) error {
	if len(id) != 6 {
		return ErrInvalid
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return ErrInvalid
		}
	}
	return nil
}

func ValidateSearch(q string) error {
	if q != strings.TrimSpace(q) || !validText(q, 2, 32) {
		return ErrInvalid
	}
	return nil
}

func ValidateEquip(in EquipInput) error {
	for _, item := range []struct{ id, kind string }{{in.ThemeID, "theme"}, {in.EmblemID, "emblem"}, {in.PatternID, "pattern"}} {
		c, ok := GetCosmetic(item.id)
		if !ok || c.Kind != item.kind {
			return ErrInvalid
		}
	}
	return nil
}

// Повторная проверка защищает от опасного текста в новостях, правленных через SQL.
var newsLinks = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://|(https?|ftp|mailto|tel|javascript|data):|www\.|//|\[[^\]]*\]\(|([a-z0-9][a-z0-9-]*\.)+[a-z]{2,63}\b)`)

func ValidNews(n News) bool {
	if !validText(n.ID, 1, 64) || !validText(n.Title, 1, 120) || !validText(n.Summary, 1, 320) || !validText(n.Category, 1, 32) || !utf8.ValidString(n.Body) || utf8.RuneCountInString(n.Body) < 1 || utf8.RuneCountInString(n.Body) > 8000 {
		return false
	}
	for _, r := range n.Body {
		if (unicode.IsControl(r) && r != '\n' && r != '\t') || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	for _, s := range []string{n.Title, n.Summary, n.Body, n.Category} {
		if strings.ContainsAny(s, "<>") || newsLinks.MatchString(s) {
			return false
		}
	}
	switch n.Art {
	case "launch", "season", "community", "market":
		return true
	}
	return false
}

func displayCosmetic(id, kind, fallback string, owned map[string]bool, admin bool) string {
	c, ok := GetCosmetic(id)
	if !ok || c.Kind != kind || !HasCosmetic(id, owned, admin) {
		return fallback
	}
	return id
}
