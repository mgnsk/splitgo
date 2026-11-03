package contract

import (
	"net/url"
	"slices"
	"time"

	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/money"
	"github.com/mgnsk/splitgo/pkg/snowflake"
)

// SettleForm records a payment from one member to another.
type SettleForm struct {
	GroupID  snowflake.ID `in:"path=group_id"`
	From     snowflake.ID `in:"form=from"`
	To       snowflake.ID `in:"form=to"`
	Amount   string       `in:"form=amount"`
	Currency string       `in:"form=currency"`
	Date     string       `in:"form=date"`
}

// ResolvedSettlement is a validated settlement.
type ResolvedSettlement struct {
	From, To snowflake.ID
	Amount   int64
	Currency money.Currency
	Date     time.Time
}

// Validate validates the form against the group's member IDs.
func (f *SettleForm) Validate(memberIDs []snowflake.ID) (*ResolvedSettlement, url.Values) {
	errs := url.Values{}

	if !slices.Contains(memberIDs, f.From) {
		errs.Set("from", "Choose who paid")
	}
	if !slices.Contains(memberIDs, f.To) {
		errs.Set("to", "Choose who received")
	}
	if f.From == f.To && f.From != 0 {
		errs.Set("to", "Payer and recipient must be different people")
	}

	date, err := time.Parse(domain.DateLayout, f.Date)
	if err != nil {
		errs.Set("date", "Invalid date")
	}

	cur, ok := money.LookupCurrency(f.Currency)
	if !ok {
		errs.Set("currency", "Unknown currency")
		return nil, errs
	}

	amount, err := money.ParseMinor(f.Amount, cur)
	if err != nil {
		errs.Set("amount", Capitalize(err.Error()))
	}

	if len(errs) > 0 {
		return nil, errs
	}

	return &ResolvedSettlement{From: f.From, To: f.To, Amount: amount, Currency: cur, Date: date}, nil
}
