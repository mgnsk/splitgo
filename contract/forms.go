// Package contract contains request forms and their validation.
package contract

import (
	"net/mail"
	"net/url"
	"strings"

	"github.com/mgnsk/splitgo/money"
	"github.com/mgnsk/splitgo/pkg/snowflake"
)

const maxNameLength = 100

// LoginForm is the login form.
type LoginForm struct {
	Email string `in:"form=email"`
	Next  string `in:"form=next"`
}

// Validate the form.
func (f *LoginForm) Validate() url.Values {
	errs := url.Values{}
	f.Email = strings.TrimSpace(f.Email)
	validateEmail(errs, "email", f.Email)
	return errs
}

// AccountForm is the account settings form.
type AccountForm struct {
	Name     string `in:"form=name"`
	Currency string `in:"form=currency"`
}

// Validate the form.
func (f *AccountForm) Validate() url.Values {
	errs := url.Values{}
	f.Name = strings.TrimSpace(f.Name)
	if len(f.Name) > maxNameLength {
		errs.Set("name", "Name is too long")
	}
	validateCurrency(errs, "currency", f.Currency)
	return errs
}

// GroupForm creates or updates a group.
type GroupForm struct {
	GroupID       snowflake.ID `in:"path=group_id"`
	Name          string       `in:"form=name"`
	Currency      string       `in:"form=currency"`
	SimplifyDebts bool         `in:"form=simplify_debts"`
}

// Validate the form.
func (f *GroupForm) Validate() url.Values {
	errs := url.Values{}
	f.Name = strings.TrimSpace(f.Name)
	switch {
	case f.Name == "":
		errs.Set("name", "Name is required")
	case len(f.Name) > maxNameLength:
		errs.Set("name", "Name is too long")
	}
	validateCurrency(errs, "currency", f.Currency)
	return errs
}

// AddMemberForm adds a member to a group by email.
type AddMemberForm struct {
	GroupID snowflake.ID `in:"path=group_id"`
	Email   string       `in:"form=email"`
}

// Validate the form.
func (f *AddMemberForm) Validate() url.Values {
	errs := url.Values{}
	f.Email = strings.TrimSpace(f.Email)
	validateEmail(errs, "email", f.Email)
	return errs
}

// GroupPath identifies a group.
type GroupPath struct {
	GroupID snowflake.ID `in:"path=group_id"`
}

// GroupPage is a group expense list page request.
type GroupPage struct {
	GroupID   snowflake.ID `in:"path=group_id"`
	AfterDate string       `in:"query=after_date"`
	AfterID   snowflake.ID `in:"query=after_id"`
}

// ExpensePath identifies an expense in a group.
type ExpensePath struct {
	GroupID   snowflake.ID `in:"path=group_id"`
	ExpenseID snowflake.ID `in:"path=expense_id"`
}

// TokenPath is a path with a token.
type TokenPath struct {
	Token string `in:"path=token"`
}

func validateEmail(errs url.Values, key, email string) {
	if email == "" {
		errs.Set(key, "Email is required")
		return
	}

	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || addr.Name != "" || len(email) > 254 {
		errs.Set(key, "Invalid email address")
	}
}

func validateCurrency(errs url.Values, key, code string) {
	if _, ok := money.LookupCurrency(code); !ok {
		errs.Set(key, "Unknown currency")
	}
}

// Capitalize upper-cases the first letter of an error message.
func Capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
