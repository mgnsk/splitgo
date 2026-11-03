// Package money implements currencies and conversion between user-entered
// decimal amounts and integer minor units (e.g. cents).
//
// All amounts are stored and summed as int64 minor units. Decimal arithmetic
// (github.com/shopspring/decimal) is used only at the edges: parsing user input
// and formatting for display.
package money

import (
	"cmp"
	"slices"
)

// Currency is an ISO 4217 currency.
type Currency struct {
	// Code is the ISO 4217 alphabetic code, e.g. "EUR".
	Code string
	// Symbol is the display symbol, e.g. "€". Falls back to Code.
	Symbol string
	// Decimals is the number of minor unit digits, e.g. 2 for EUR, 0 for JPY.
	Decimals int32
}

// DefaultCurrency is the default currency code.
const DefaultCurrency = "EUR"

var currencies = map[string]Currency{}

func init() {
	for _, c := range []Currency{
		{"AED", "AED", 2},
		{"ARS", "ARS", 2},
		{"AUD", "A$", 2},
		{"BGN", "BGN", 2},
		{"BHD", "BHD", 3},
		{"BRL", "R$", 2},
		{"CAD", "CA$", 2},
		{"CHF", "CHF", 2},
		{"CLP", "CLP", 0},
		{"CNY", "CN¥", 2},
		{"CZK", "CZK", 2},
		{"DKK", "DKK", 2},
		{"EUR", "€", 2},
		{"GBP", "£", 2},
		{"HKD", "HK$", 2},
		{"HUF", "HUF", 2},
		{"IDR", "IDR", 2},
		{"ILS", "₪", 2},
		{"INR", "₹", 2},
		{"ISK", "ISK", 0},
		{"JOD", "JOD", 3},
		{"JPY", "¥", 0},
		{"KRW", "₩", 0},
		{"KWD", "KWD", 3},
		{"MXN", "MX$", 2},
		{"MYR", "MYR", 2},
		{"NOK", "NOK", 2},
		{"NZD", "NZ$", 2},
		{"OMR", "OMR", 3},
		{"PHP", "₱", 2},
		{"PLN", "PLN", 2},
		{"RON", "RON", 2},
		{"SEK", "SEK", 2},
		{"SGD", "SGD", 2},
		{"THB", "THB", 2},
		{"TND", "TND", 3},
		{"TRY", "TRY", 2},
		{"TWD", "NT$", 2},
		{"UAH", "UAH", 2},
		{"USD", "$", 2},
		{"VND", "₫", 0},
		{"ZAR", "ZAR", 2},
	} {
		currencies[c.Code] = c
	}
}

// LookupCurrency returns the currency for an ISO 4217 code.
func LookupCurrency(code string) (Currency, bool) {
	c, ok := currencies[code]
	return c, ok
}

// MustCurrency returns the currency for code or panics.
func MustCurrency(code string) Currency {
	c, ok := LookupCurrency(code)
	if !ok {
		panic("unknown currency: " + code)
	}
	return c
}

// Currencies returns all supported currencies sorted by code.
func Currencies() []Currency {
	list := make([]Currency, 0, len(currencies))
	for _, c := range currencies {
		list = append(list, c)
	}
	slices.SortFunc(list, func(a, b Currency) int { return cmp.Compare(a.Code, b.Code) })
	return list
}
