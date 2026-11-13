package domain

import (
	"time"

	"github.com/mgnsk/splitgo/split"
	"github.com/mgnsk/webkit/snowflake"
)

// ExpenseKind distinguishes expenses from settlements.
type ExpenseKind string

// Expense kinds.
const (
	KindExpense    ExpenseKind = "expense"
	KindSettlement ExpenseKind = "settlement"
)

// DateLayout is the layout of expense dates.
const DateLayout = time.DateOnly

// Expense is a shared expense or a settlement within a group.
//
// A settlement "A pays B" is stored as an expense paid by A with B as the only
// participant owing the full amount, which cancels A's debt to B.
type Expense struct {
	ID           snowflake.ID
	GroupID      snowflake.ID
	Kind         ExpenseKind
	Description  string
	Amount       int64 // minor units
	Currency     string
	SplitType    split.Type
	PaidBy       snowflake.ID
	AddedBy      snowflake.ID
	Date         time.Time
	UpdatedAt    time.Time
	Participants []Participant
}

// Participant is a person's share of an expense.
type Participant struct {
	UserID snowflake.ID
	// Share is the participant's part of the total in minor units.
	Share int64
	// SplitValue is the original split input, e.g. a percentage.
	SplitValue string
}

// Shares returns participant shares keyed by user ID.
func (e *Expense) Shares() map[snowflake.ID]int64 {
	m := make(map[snowflake.ID]int64, len(e.Participants))
	for _, p := range e.Participants {
		m[p.UserID] += p.Share
	}
	return m
}

// ShareOf returns the share of user.
func (e *Expense) ShareOf(user snowflake.ID) int64 {
	return e.Shares()[user]
}

// ParticipantIDs returns IDs of participants with a non-zero share.
func (e *Expense) ParticipantIDs() []snowflake.ID {
	var ids []snowflake.ID
	for _, p := range e.Participants {
		if p.Share > 0 {
			ids = append(ids, p.UserID)
		}
	}
	return ids
}

// Involves reports whether the user paid or participates.
func (e *Expense) Involves(user snowflake.ID) bool {
	return e.PaidBy == user || e.ShareOf(user) > 0
}

// Net returns how the expense changes user's balance: positive means others
// owe the user more, negative means the user owes more.
func (e *Expense) Net(user snowflake.ID) int64 {
	var net int64
	if e.PaidBy == user {
		net += e.Amount
	}
	return net - e.ShareOf(user)
}
