package money

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// MaxMinor is the largest accepted amount in minor units for a single
// expense. It leaves ample headroom so that summing many amounts can not
// overflow int64.
const MaxMinor int64 = 1_000_000_000_000_00 // 10^14

// Parse errors.
var (
	ErrEmpty       = errors.New("amount is required")
	ErrInvalid     = errors.New("invalid amount")
	ErrNegative    = errors.New("amount must not be negative")
	ErrTooPrecise  = errors.New("too many decimal places")
	ErrTooLarge    = errors.New("amount is too large")
	ErrNotPositive = errors.New("amount must be greater than zero")
)

// ParseDecimal parses a user-entered non-negative decimal number.
//
// Accepted: "12", "12.5", "12,50", "1 234,56", "1,234.56", "1.234,56".
// When both '.' and ',' occur, the last one is the decimal separator and the
// other is a thousands separator. When only one kind occurs once, it is the
// decimal separator; when it occurs more than once, it is a thousands separator.
func ParseDecimal(s string) (decimal.Decimal, error) {
	s = strings.Map(func(r rune) rune {
		switch r {
		case ' ', ' ', ' ', '\'', '_':
			return -1 // drop grouping characters
		}
		return r
	}, strings.TrimSpace(s))

	if s == "" {
		return decimal.Zero, ErrEmpty
	}

	if strings.HasPrefix(s, "-") {
		return decimal.Zero, ErrNegative
	}

	lastDot := strings.LastIndexByte(s, '.')
	lastComma := strings.LastIndexByte(s, ',')

	var decSep, thouSep string
	switch {
	case lastDot >= 0 && lastComma >= 0:
		if lastDot > lastComma {
			decSep, thouSep = ".", ","
		} else {
			decSep, thouSep = ",", "."
		}
	case lastDot >= 0:
		if strings.Count(s, ".") == 1 {
			decSep = "."
		} else {
			thouSep = "."
		}
	case lastComma >= 0:
		if strings.Count(s, ",") == 1 {
			decSep = ","
		} else {
			thouSep = ","
		}
	}

	if thouSep != "" {
		s = strings.ReplaceAll(s, thouSep, "")
	}
	if decSep != "" {
		if strings.Count(s, decSep) != 1 {
			return decimal.Zero, ErrInvalid
		}
		s = strings.Replace(s, decSep, ".", 1)
	}

	// Only digits and a single '.' with digits on at least one side.
	digits := 0
	for i, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r == '.' && i == strings.IndexByte(s, '.'):
		default:
			return decimal.Zero, ErrInvalid
		}
	}
	if digits == 0 || digits > 30 {
		return decimal.Zero, ErrInvalid
	}

	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, ErrInvalid
	}

	return d, nil
}

// ToMinor converts a decimal amount to minor units of c. The amount must be
// representable exactly, i.e. have at most c.Decimals decimal places.
func ToMinor(d decimal.Decimal, c Currency) (int64, error) {
	scaled := d.Shift(c.Decimals)
	if !scaled.Equal(scaled.Truncate(0)) {
		return 0, ErrTooPrecise
	}
	if scaled.Abs().GreaterThan(decimal.NewFromInt(MaxMinor)) {
		return 0, ErrTooLarge
	}
	return scaled.IntPart(), nil
}

// FromMinor converts minor units of c to a decimal amount.
func FromMinor(minor int64, c Currency) decimal.Decimal {
	return decimal.New(minor, -c.Decimals)
}

// ParseMinor parses user input into strictly positive minor units of c.
func ParseMinor(s string, c Currency) (int64, error) {
	d, err := ParseDecimal(s)
	if err != nil {
		return 0, err
	}

	minor, err := ToMinor(d, c)
	if err != nil {
		return 0, err
	}

	if minor <= 0 {
		return 0, ErrNotPositive
	}

	return minor, nil
}

// FormatPlain formats minor units as a plain decimal string with exactly
// c.Decimals decimal places and no grouping, suitable for input values.
func FormatPlain(minor int64, c Currency) string {
	return FromMinor(minor, c).StringFixed(c.Decimals)
}

// Format formats minor units for display, e.g. "€1,234.50", "-¥1,200",
// "1,234.500 KWD".
func Format(minor int64, c Currency) string {
	neg := minor < 0
	abs := FromMinor(minor, c).Abs().StringFixed(c.Decimals)

	intPart, frac, _ := strings.Cut(abs, ".")
	var b strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if frac != "" {
		b.WriteByte('.')
		b.WriteString(frac)
	}

	sign := ""
	if neg {
		sign = "-"
	}

	if c.Symbol == "" || c.Symbol == c.Code {
		return fmt.Sprintf("%s%s %s", sign, b.String(), c.Code)
	}

	return sign + c.Symbol + b.String()
}
