package transfer_sql

import "time"

type Transfer struct {
	ID            int
	SenderID      int
	RecipientID   int
	SenderName    string
	RecipientName string
	Amount        float64
	Status        string
	CreatedAt     time.Time
}
