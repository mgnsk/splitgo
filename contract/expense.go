package contract

import (
	"cmp"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/money"
	"github.com/mgnsk/webkit/snowflake"
	"github.com/mgnsk/splitgo/split"
	"github.com/shopspring/decimal"
)

// Expense form modes.
const (
	ModeSimple   = "simple"
	ModeAdvanced = "advanced"
)

const maxDescriptionLength = 200

// ExpenseForm is the add/edit expense form.
//
// In simple mode the split is chosen with a preset (two-person groups) or as
// "paid by X, split equally among everyone" (larger groups). In advanced mode
// the payer, split type, participants and per-person values are explicit.
type ExpenseForm struct {
	Description string
	Amount      string
	Currency    string
	Date        string
	Mode        string
	Preset      split.Preset
	PaidBy      snowflake.ID
	SplitType   split.Type
	// Included are the participants of equal and adjustment splits.
	Included map[snowflake.ID]bool
	// Values are per-person inputs keyed by user ID: percentages, shares,
	// exact amounts or adjustments depending on SplitType.
	Values map[snowflake.ID]string
}

// ParseExpenseForm parses the form from request form values.
func ParseExpenseForm(form url.Values) *ExpenseForm {
	f := &ExpenseForm{
		Description: strings.TrimSpace(form.Get("description")),
		Amount:      strings.TrimSpace(form.Get("amount")),
		Currency:    form.Get("currency"),
		Date:        form.Get("date"),
		Mode:        form.Get("mode"),
		Preset:      split.Preset(form.Get("preset")),
		SplitType:   split.Type(form.Get("split_type")),
		Included:    map[snowflake.ID]bool{},
		Values:      map[snowflake.ID]string{},
	}

	if f.Mode != ModeAdvanced {
		f.Mode = ModeSimple
	}

	if !f.SplitType.Valid() {
		f.SplitType = split.Equal
	}

	if id, ok := parseID(form.Get("paid_by")); ok {
		f.PaidBy = id
	}

	for _, s := range form["included"] {
		if id, ok := parseID(s); ok {
			f.Included[id] = true
		}
	}

	for key, vals := range form {
		if rest, ok := strings.CutPrefix(key, "value_"); ok && len(vals) > 0 {
			if id, ok := parseID(rest); ok {
				f.Values[id] = strings.TrimSpace(vals[0])
			}
		}
	}

	return f
}

// NewExpenseForm returns a form with defaults for a new expense.
func NewExpenseForm(group *domain.Group, me snowflake.ID, people []*domain.User, today time.Time) *ExpenseForm {
	f := &ExpenseForm{
		Currency:  group.Currency,
		Date:      today.Format(domain.DateLayout),
		Mode:      ModeSimple,
		Preset:    split.YouPaidEqual,
		PaidBy:    me,
		SplitType: split.Equal,
		Included:  map[snowflake.ID]bool{},
		Values:    map[snowflake.ID]string{},
	}
	for _, p := range people {
		f.Included[p.ID] = true
	}
	return f
}

// ExpenseFormFromExpense returns a form populated from an existing expense.
// It opens in simple mode when the split can be expressed there.
func ExpenseFormFromExpense(e *domain.Expense, me snowflake.ID, people []*domain.User) *ExpenseForm {
	cur := money.MustCurrency(e.Currency)

	f := &ExpenseForm{
		Description: e.Description,
		Amount:      money.FormatPlain(e.Amount, cur),
		Currency:    e.Currency,
		Date:        e.Date.Format(domain.DateLayout),
		Mode:        ModeAdvanced,
		PaidBy:      e.PaidBy,
		SplitType:   e.SplitType,
		Included:    map[snowflake.ID]bool{},
		Values:      map[snowflake.ID]string{},
	}

	for _, p := range e.Participants {
		f.Included[p.UserID] = true
		f.Values[p.UserID] = p.SplitValue
	}

	ids := personIDs(people)
	participants := e.ParticipantIDs()

	if len(people) == 2 {
		if other, ok := otherPerson(me, people); ok {
			if preset, ok := split.Detect(e.SplitType, e.PaidBy, participants, me, other); ok {
				f.Mode = ModeSimple
				f.Preset = preset
			}
		}
	} else if split.IsEqualAmongAll(e.SplitType, participants, ids) {
		f.Mode = ModeSimple
	}

	if f.Preset == "" {
		f.Preset = split.YouPaidEqual
	}

	return f
}

// ResolvedSplit is a computed split.
type ResolvedSplit struct {
	PaidBy       snowflake.ID
	SplitType    split.Type
	Participants []domain.Participant
}

// Shares returns shares keyed by user ID.
func (s *ResolvedSplit) Shares() map[snowflake.ID]int64 {
	m := map[snowflake.ID]int64{}
	for _, p := range s.Participants {
		m[p.UserID] = p.Share
	}
	return m
}

// ResolvedExpense is a validated expense form.
type ResolvedExpense struct {
	ResolvedSplit
	Description string
	Amount      int64
	Currency    money.Currency
	Date        time.Time
}

// Validate validates the whole form and computes the split. people must be
// sorted by ID and contain me.
func (f *ExpenseForm) Validate(me snowflake.ID, people []*domain.User) (*ResolvedExpense, url.Values) {
	errs := url.Values{}

	switch {
	case f.Description == "":
		errs.Set("description", "Description is required")
	case len(f.Description) > maxDescriptionLength:
		errs.Set("description", "Description is too long")
	}

	date, err := time.Parse(domain.DateLayout, f.Date)
	if err != nil {
		errs.Set("date", "Invalid date")
	}

	cur, amount, ok := f.parseAmount(errs)
	if !ok {
		return nil, errs
	}

	resolved, splitErrs := f.ResolveSplit(me, people, amount, cur)
	for k, v := range splitErrs {
		errs[k] = v
	}

	if len(errs) > 0 {
		return nil, errs
	}

	return &ResolvedExpense{
		ResolvedSplit: *resolved,
		Description:   f.Description,
		Amount:        amount,
		Currency:      cur,
		Date:          date,
	}, nil
}

// ParseAmount parses the currency and amount fields.
func (f *ExpenseForm) ParseAmount() (money.Currency, int64, url.Values) {
	errs := url.Values{}
	cur, amount, _ := f.parseAmount(errs)
	return cur, amount, errs
}

func (f *ExpenseForm) parseAmount(errs url.Values) (money.Currency, int64, bool) {
	cur, ok := money.LookupCurrency(f.Currency)
	if !ok {
		errs.Set("currency", "Unknown currency")
		return cur, 0, false
	}

	amount, err := money.ParseMinor(f.Amount, cur)
	if err != nil {
		errs.Set("amount", Capitalize(err.Error()))
		return cur, 0, false
	}

	return cur, amount, true
}

// ResolveSplit computes the split of amount minor units. people must be
// sorted by ID and contain me. Errors are keyed by "split", "paid_by",
// "preset" or "value_<id>".
func (f *ExpenseForm) ResolveSplit(me snowflake.ID, people []*domain.User, amount int64, cur money.Currency) (*ResolvedSplit, url.Values) {
	errs := url.Values{}
	ids := personIDs(people)

	if !slices.Contains(ids, me) {
		errs.Set("split", "You are not a member of this group")
		return nil, errs
	}

	var (
		payer        snowflake.ID
		typ          = split.Equal
		participants []snowflake.ID
		values       []decimal.Decimal
		splitValues  []string
	)

	switch {
	case f.Mode == ModeSimple && len(people) == 2:
		other, _ := otherPerson(me, people)
		plan, ok := split.Resolve(f.Preset, me, other)
		if !ok {
			errs.Set("preset", "Choose how to split")
			return nil, errs
		}
		payer = plan.Payer
		participants = sortedIDs(plan.Participants)

	case f.Mode == ModeSimple:
		payer = f.PaidBy
		participants = ids

	default:
		payer = f.PaidBy
		typ = f.SplitType

		switch typ {
		case split.Equal, split.Adjustment:
			for _, id := range ids {
				if f.Included[id] {
					participants = append(participants, id)
				}
			}
		default:
			participants = ids
		}

		if typ != split.Equal {
			values = make([]decimal.Decimal, len(participants))
			splitValues = make([]string, len(participants))
			for i, id := range participants {
				v, s, err := parseSplitValue(typ, f.Values[id], cur)
				if err != nil {
					errs.Set(valueKey(id), Capitalize(err.Error()))
					continue
				}
				values[i], splitValues[i] = v, s
			}
		}
	}

	if !slices.Contains(ids, payer) {
		errs.Set("paid_by", "Choose who paid")
	}

	if len(participants) == 0 {
		errs.Set("split", Capitalize(split.ErrNoParticipants.Error()))
	}

	if len(errs) > 0 {
		return nil, errs
	}

	if values == nil {
		values = make([]decimal.Decimal, len(participants))
	}

	// Rounding leftovers go to participants in order, so put the payer first:
	// the payer absorbs rounding and debtors never owe an extra minor unit.
	if i := slices.Index(participants, payer); i > 0 {
		participants = slices.Clone(participants)
		values = slices.Clone(values)
		pv := values[i]
		copy(participants[1:i+1], participants[:i])
		copy(values[1:i+1], values[:i])
		participants[0], values[0] = payer, pv
		if splitValues != nil {
			splitValues = slices.Clone(splitValues)
			sv := splitValues[i]
			copy(splitValues[1:i+1], splitValues[:i])
			splitValues[0] = sv
		}
	}

	shares, err := split.Compute(amount, typ, values)
	if err != nil {
		errs.Set("split", splitErrorMessage(err, typ, cur))
		return nil, errs
	}

	result := &ResolvedSplit{PaidBy: payer, SplitType: typ}
	for i, id := range participants {
		if shares[i] == 0 {
			continue
		}
		p := domain.Participant{UserID: id, Share: shares[i]}
		if splitValues != nil {
			p.SplitValue = splitValues[i]
		}
		result.Participants = append(result.Participants, p)
	}

	slices.SortFunc(result.Participants, func(a, b domain.Participant) int {
		return cmp.Compare(a.UserID, b.UserID)
	})

	return result, nil
}

func valueKey(id snowflake.ID) string {
	return "value_" + id.String()
}

// ValueKey returns the form field name of a person's split value.
func ValueKey(id snowflake.ID) string {
	return valueKey(id)
}

// parseSplitValue parses a per-person split input. It returns the value for
// split.Compute and its normalized string form for storage.
func parseSplitValue(typ split.Type, s string, cur money.Currency) (decimal.Decimal, string, error) {
	if s == "" {
		return decimal.Zero, "", nil
	}

	switch typ {
	case split.Percentage, split.Shares:
		d, err := money.ParseDecimal(s)
		if err != nil {
			return decimal.Zero, "", err
		}
		return d, d.String(), nil

	case split.Exact:
		d, err := money.ParseDecimal(s)
		if err != nil {
			return decimal.Zero, "", err
		}
		minor, err := money.ToMinor(d, cur)
		if err != nil {
			return decimal.Zero, "", err
		}
		return decimal.NewFromInt(minor), money.FormatPlain(minor, cur), nil

	case split.Adjustment:
		neg := false
		if rest, ok := strings.CutPrefix(s, "-"); ok {
			neg, s = true, rest
		} else {
			s = strings.TrimPrefix(s, "+")
		}
		d, err := money.ParseDecimal(s)
		if err != nil {
			return decimal.Zero, "", err
		}
		minor, err := money.ToMinor(d, cur)
		if err != nil {
			return decimal.Zero, "", err
		}
		if neg {
			minor = -minor
		}
		return decimal.NewFromInt(minor), money.FormatPlain(minor, cur), nil
	}

	return decimal.Zero, "", nil
}

func splitErrorMessage(err error, typ split.Type, cur money.Currency) string {
	if se, ok := errors.AsType[*split.SumError](err); ok {
		switch typ {
		case split.Percentage:
			return fmt.Sprintf("Percentages add up to %s%%, not 100%%", se.Got.String())
		case split.Exact:
			got, want := se.Got.IntPart(), se.Want.IntPart()
			if got < want {
				return fmt.Sprintf("Amounts add up to %s, %s left to assign",
					money.Format(got, cur), money.Format(want-got, cur))
			}
			return fmt.Sprintf("Amounts add up to %s, %s over the total",
				money.Format(got, cur), money.Format(got-want, cur))
		}
	}
	return Capitalize(err.Error())
}

func parseID(s string) (snowflake.ID, bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return snowflake.ID(n), true
}

func personIDs(people []*domain.User) []snowflake.ID {
	ids := make([]snowflake.ID, len(people))
	for i, p := range people {
		ids[i] = p.ID
	}
	return ids
}

func sortedIDs(ids []snowflake.ID) []snowflake.ID {
	ids = slices.Clone(ids)
	slices.SortFunc(ids, cmp.Compare)
	return ids
}

func otherPerson(me snowflake.ID, people []*domain.User) (snowflake.ID, bool) {
	for _, p := range people {
		if p.ID != me {
			return p.ID, true
		}
	}
	return 0, false
}

// SwitchToAdvanced switches the form to advanced mode, carrying over the
// payer and participants chosen in simple mode.
func (f *ExpenseForm) SwitchToAdvanced(me snowflake.ID, people []*domain.User) {
	if f.Mode == ModeAdvanced {
		return
	}

	f.Mode = ModeAdvanced
	f.SplitType = split.Equal
	f.Included = map[snowflake.ID]bool{}

	if len(people) == 2 {
		other, _ := otherPerson(me, people)
		if plan, ok := split.Resolve(f.Preset, me, other); ok {
			f.PaidBy = plan.Payer
			for _, id := range plan.Participants {
				f.Included[id] = true
			}
			return
		}
	}

	for _, p := range people {
		f.Included[p.ID] = true
	}
}
