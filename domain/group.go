package domain

import (
	"github.com/mgnsk/splitgo/pkg/snowflake"
)

// GroupKind tells ordinary groups from the hidden groups of two friends.
type GroupKind string

// Group kinds.
const (
	// GroupKindGroup is a group people create and invite others to.
	GroupKindGroup GroupKind = "group"
	// GroupKindDirect holds the non-group expenses of two friends. It is
	// never shown as a group.
	GroupKindDirect GroupKind = "direct"
)

// Group is a group of people sharing expenses.
type Group struct {
	ID            snowflake.ID
	Kind          GroupKind
	Name          string
	Currency      string
	SimplifyDebts bool
	InviteCode    string
	CreatedBy     snowflake.ID
}

// Direct reports whether g holds the non-group expenses of two friends.
func (g *Group) Direct() bool {
	return g.Kind == GroupKindDirect
}
