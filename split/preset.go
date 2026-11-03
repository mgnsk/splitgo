package split

import "slices"

// Preset is a quick split option between the current user and one other
// person, as offered by the simple expense form.
type Preset string

// Presets for two-person expenses.
const (
	// YouPaidEqual - you paid, split equally.
	YouPaidEqual Preset = "you-paid-equal"
	// YouOwedFull - you paid, the other person owes the full amount.
	YouOwedFull Preset = "you-owed-full"
	// TheyPaidEqual - the other person paid, split equally.
	TheyPaidEqual Preset = "they-paid-equal"
	// TheyOwedFull - the other person paid, you owe the full amount.
	TheyOwedFull Preset = "they-owed-full"
)

// Presets lists all presets in display order.
var Presets = []Preset{YouPaidEqual, YouOwedFull, TheyPaidEqual, TheyOwedFull}

// Valid reports whether p is a known preset.
func (p Preset) Valid() bool {
	return slices.Contains(Presets, p)
}

// Plan is a resolved payer and equal-split participant set.
type Plan[ID comparable] struct {
	Payer        ID
	Participants []ID
}

// Resolve maps the preset to a payer and the participants of an Equal split.
// Participants are returned as (me, other) filtered by inclusion; callers
// should sort them in their canonical order before computing shares.
func Resolve[ID comparable](p Preset, me, other ID) (Plan[ID], bool) {
	switch p {
	case YouPaidEqual:
		return Plan[ID]{Payer: me, Participants: []ID{me, other}}, true
	case YouOwedFull:
		return Plan[ID]{Payer: me, Participants: []ID{other}}, true
	case TheyPaidEqual:
		return Plan[ID]{Payer: other, Participants: []ID{me, other}}, true
	case TheyOwedFull:
		return Plan[ID]{Payer: other, Participants: []ID{me}}, true
	default:
		return Plan[ID]{}, false
	}
}

// Detect returns the preset matching an existing Equal split between me and
// other. participants are the users with a non-zero share. It returns false
// when the expense can only be represented in the advanced view.
func Detect[ID comparable](t Type, payer ID, participants []ID, me, other ID) (Preset, bool) {
	if t != Equal || me == other {
		return "", false
	}

	for _, p := range participants {
		if p != me && p != other {
			return "", false
		}
	}

	hasMe := slices.Contains(participants, me)
	hasOther := slices.Contains(participants, other)

	switch {
	case payer == me && hasMe && hasOther && len(participants) == 2:
		return YouPaidEqual, true
	case payer == me && !hasMe && hasOther && len(participants) == 1:
		return YouOwedFull, true
	case payer == other && hasMe && hasOther && len(participants) == 2:
		return TheyPaidEqual, true
	case payer == other && hasMe && !hasOther && len(participants) == 1:
		return TheyOwedFull, true
	default:
		return "", false
	}
}

// IsEqualAmongAll reports whether a split is an Equal split among all members,
// which is what the simple view offers for groups with more than two members.
func IsEqualAmongAll[ID comparable](t Type, participants, members []ID) bool {
	if t != Equal || len(participants) != len(members) {
		return false
	}
	for _, m := range members {
		if !slices.Contains(participants, m) {
			return false
		}
	}
	return true
}
