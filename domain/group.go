package domain

import (
	"github.com/mgnsk/splitgo/pkg/snowflake"
)

// Group is a group of people sharing expenses.
type Group struct {
	ID            snowflake.ID
	Name          string
	Currency      string
	SimplifyDebts bool
	InviteCode    string
	CreatedBy     snowflake.ID
}
