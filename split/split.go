// Package split computes how an expense total is divided between participants.
//
// All results are integer minor units that always sum exactly to the total.
// Rounding uses the largest remainder method: every participant first gets the
// floor of their exact share, then the leftover minor units go one each to the
// participants with the largest fractional remainders. Ties are broken by input
// order, so callers should pass participants in a stable order (e.g. by user ID).
package split

import (
	"errors"
	"fmt"
	"math/big"
	"slices"

	"github.com/shopspring/decimal"
)

// Type is a split type.
type Type string

// Split types.
const (
	// Equal splits the total equally between participants. Values are ignored.
	Equal Type = "equal"
	// Percentage splits by percentages. Values must sum to exactly 100.
	Percentage Type = "percentage"
	// Shares splits proportionally to non-negative weights.
	Shares Type = "shares"
	// Exact assigns exact amounts in minor units. Values must sum to the total.
	Exact Type = "exact"
	// Adjustment splits equally after adding per-participant adjustments
	// in minor units: share_i = (total - Σadj)/n + adj_i.
	Adjustment Type = "adjustment"
)

// Types lists all split types in display order.
var Types = []Type{Equal, Percentage, Shares, Exact, Adjustment}

// Valid reports whether t is a known split type.
func (t Type) Valid() bool {
	return slices.Contains(Types, t)
}

// Label returns a human readable name.
func (t Type) Label() string {
	switch t {
	case Equal:
		return "Equally"
	case Percentage:
		return "By percentage"
	case Shares:
		return "By shares"
	case Exact:
		return "Exact amounts"
	case Adjustment:
		return "Adjustment"
	default:
		return string(t)
	}
}

// Errors returned by Compute.
var (
	ErrUnknownType      = errors.New("unknown split type")
	ErrTotalNotPositive = errors.New("total must be greater than zero")
	ErrNoParticipants   = errors.New("select at least one person")
	ErrNegativeValue    = errors.New("values must not be negative")
	ErrPercentSum       = errors.New("percentages must add up to 100")
	ErrSharesZero       = errors.New("total shares must be greater than zero")
	ErrNotWholeUnits    = errors.New("amounts must be whole minor units")
	ErrExactSum         = errors.New("amounts must add up to the total")
	ErrAdjustmentExceed = errors.New("adjustments exceed the total")
	ErrNegativeShare    = errors.New("adjustments result in a negative share")
)

// SumError is returned when values do not add up as required.
// It wraps ErrPercentSum or ErrExactSum.
type SumError struct {
	Err  error
	Want decimal.Decimal
	Got  decimal.Decimal
}

func (e *SumError) Error() string {
	return fmt.Sprintf("%s (got %s, want %s)", e.Err, e.Got, e.Want)
}

func (e *SumError) Unwrap() error { return e.Err }

var hundred = decimal.NewFromInt(100)

// Compute splits total (minor units, > 0) between len(values) participants.
// The meaning of values depends on t (see Type constants). The result has the
// same length and order as values and always sums to total.
func Compute(total int64, t Type, values []decimal.Decimal) ([]int64, error) {
	if !t.Valid() {
		return nil, ErrUnknownType
	}

	if total <= 0 {
		return nil, ErrTotalNotPositive
	}

	if len(values) == 0 {
		return nil, ErrNoParticipants
	}

	if t != Equal && t != Adjustment {
		for _, v := range values {
			if v.IsNegative() {
				return nil, ErrNegativeValue
			}
		}
	}

	switch t {
	case Equal:
		return apportion(total, ones(len(values))), nil

	case Percentage:
		if sum := decimal.Sum(decimal.Zero, values...); !sum.Equal(hundred) {
			return nil, &SumError{Err: ErrPercentSum, Want: hundred, Got: sum}
		}
		return apportion(total, values), nil

	case Shares:
		if decimal.Sum(decimal.Zero, values...).IsZero() {
			return nil, ErrSharesZero
		}
		return apportion(total, values), nil

	case Exact:
		result := make([]int64, len(values))
		var sum int64
		for i, v := range values {
			if !v.IsInteger() {
				return nil, ErrNotWholeUnits
			}
			if v.GreaterThan(decimal.NewFromInt(total)) {
				return nil, &SumError{Err: ErrExactSum, Want: decimal.NewFromInt(total), Got: decimal.Sum(decimal.Zero, values...)}
			}
			result[i] = v.IntPart()
			sum += result[i]
		}
		if sum != total {
			return nil, &SumError{Err: ErrExactSum, Want: decimal.NewFromInt(total), Got: decimal.NewFromInt(sum)}
		}
		return result, nil

	case Adjustment:
		totalDec := decimal.NewFromInt(total)
		adjSum := decimal.Zero
		for _, v := range values {
			if !v.IsInteger() {
				return nil, ErrNotWholeUnits
			}
			if v.Abs().GreaterThan(totalDec) {
				return nil, ErrAdjustmentExceed
			}
			adjSum = adjSum.Add(v)
		}
		rest := totalDec.Sub(adjSum)
		if rest.IsNegative() {
			return nil, ErrAdjustmentExceed
		}
		result := apportion(rest.IntPart(), ones(len(values)))
		for i, v := range values {
			result[i] += v.IntPart()
			if result[i] < 0 {
				return nil, ErrNegativeShare
			}
		}
		return result, nil
	}

	return nil, ErrUnknownType
}

func ones(n int) []decimal.Decimal {
	v := make([]decimal.Decimal, n)
	for i := range v {
		v[i] = decimal.NewFromInt(1)
	}
	return v
}

// apportion divides total proportionally to non-negative weights with a
// non-zero sum using the largest remainder method and exact integer arithmetic.
func apportion(total int64, weights []decimal.Decimal) []int64 {
	// Scale all weights to integers with a common exponent.
	minExp := int32(0)
	for _, w := range weights {
		minExp = min(minExp, w.Exponent())
	}

	ints := make([]*big.Int, len(weights))
	sum := new(big.Int)
	for i, w := range weights {
		ints[i] = w.Shift(-minExp).BigInt()
		sum.Add(sum, ints[i])
	}

	bigTotal := big.NewInt(total)
	result := make([]int64, len(weights))
	rems := make([]*big.Int, len(weights))
	var allocated int64

	for i, w := range ints {
		num := new(big.Int).Mul(bigTotal, w)
		q, r := new(big.Int).QuoRem(num, sum, new(big.Int))
		result[i] = q.Int64()
		rems[i] = r
		allocated += result[i]
	}

	// Hand out leftover units to the largest remainders, ties by index.
	order := make([]int, len(weights))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return rems[b].Cmp(rems[a])
	})

	for k := int64(0); k < total-allocated; k++ {
		result[order[k]]++
	}

	return result
}
