package contacts_sql

type ContactItem struct {
	PlayerID string
	Username string
}

type UserSearchResult struct {
	PlayerID  string
	Username  string
	IsContact bool
}
