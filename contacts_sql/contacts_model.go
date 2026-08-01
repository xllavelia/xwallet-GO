package contacts_sql

type ContactItem struct {
	PlayerID string `json:"playerId"`
	Username string `json:"username"`
}

type UserSearchResult struct {
	PlayerID  string `json:"playerId"`
	Username  string `json:"username"`
	IsContact bool   `json:"isContact"`
}
