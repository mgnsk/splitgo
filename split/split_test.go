package split_test

import (
	"math/rand/v2"
	"testing"

	"github.com/mgnsk/splitgo/split"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func ds(ss ...string) []decimal.Decimal {
	out := make([]decimal.Decimal, len(ss))
	for i, s := range ss {
		out[i] = d(s)
	}
	return out
}

func zeros(n int) []decimal.Decimal { return make([]decimal.Decimal, n) }

func sum(xs []int64) (s int64) {
	for _, x := range xs {
		s += x
	}
	return s
}

var _ = Describe("Compute", func() {
	DescribeTable("valid splits",
		func(total int64, t split.Type, values []decimal.Decimal, expected []int64) {
			result, err := split.Compute(total, t, values)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(expected))
			Expect(sum(result)).To(Equal(total))
		},

		// Equal.
		Entry("equal even", int64(1000), split.Equal, zeros(2), []int64{500, 500}),
		Entry("equal single", int64(999), split.Equal, zeros(1), []int64{999}),
		Entry("equal 100/3 first gets the extra cent", int64(100), split.Equal, zeros(3), []int64{34, 33, 33}),
		Entry("equal 200/3 first two get extra cents", int64(200), split.Equal, zeros(3), []int64{67, 67, 66}),
		Entry("equal 10001/3", int64(10001), split.Equal, zeros(3), []int64{3334, 3334, 3333}),
		Entry("equal 1001/2", int64(1001), split.Equal, zeros(2), []int64{501, 500}),
		Entry("equal 1 cent among 3", int64(1), split.Equal, zeros(3), []int64{1, 0, 0}),
		Entry("equal 2 cents among 3", int64(2), split.Equal, zeros(3), []int64{1, 1, 0}),
		Entry("equal ignores values", int64(100), split.Equal, ds("5", "0"), []int64{50, 50}),
		Entry("equal max amount among 7", int64(100_000_000_000_000), split.Equal, zeros(7),
			[]int64{14285714285715, 14285714285715, 14285714285714, 14285714285714, 14285714285714, 14285714285714, 14285714285714}),

		// Percentage.
		Entry("percentage 50/50", int64(1000), split.Percentage, ds("50", "50"), []int64{500, 500}),
		Entry("percentage 33.33/33.33/33.34", int64(10000), split.Percentage, ds("33.33", "33.33", "33.34"), []int64{3333, 3333, 3334}),
		Entry("percentage 33.33/33.33/33.34 of 100", int64(100), split.Percentage, ds("33.33", "33.33", "33.34"), []int64{33, 33, 34}),
		Entry("percentage 70/30 of 999", int64(999), split.Percentage, ds("70", "30"), []int64{699, 300}),
		Entry("percentage with zero participant", int64(1000), split.Percentage, ds("100", "0"), []int64{1000, 0}),
		Entry("percentage high precision", int64(3), split.Percentage, ds("33.3333", "33.3333", "33.3334"), []int64{1, 1, 1}),
		Entry("percentage remainder goes to largest fraction", int64(10), split.Percentage, ds("15", "85"), []int64{2, 8}),
		Entry("percentage 1 cent", int64(1), split.Percentage, ds("40", "60"), []int64{0, 1}),

		// Shares.
		Entry("shares 1:2", int64(300), split.Shares, ds("1", "2"), []int64{100, 200}),
		Entry("shares 1:2 uneven", int64(100), split.Shares, ds("1", "2"), []int64{33, 67}),
		Entry("shares 1:1:1 behaves like equal", int64(100), split.Shares, ds("1", "1", "1"), []int64{34, 33, 33}),
		Entry("shares fractional weights", int64(1000), split.Shares, ds("0.5", "1.5"), []int64{250, 750}),
		Entry("shares zero weight gets nothing", int64(101), split.Shares, ds("1", "0", "1"), []int64{51, 0, 50}),
		Entry("shares large weights", int64(1000), split.Shares, ds("1000000", "3000000"), []int64{250, 750}),

		// Exact.
		Entry("exact", int64(1000), split.Exact, ds("250", "750"), []int64{250, 750}),
		Entry("exact with zero", int64(1000), split.Exact, ds("1000", "0"), []int64{1000, 0}),
		Entry("exact trailing zero decimals", int64(1000), split.Exact, ds("250.00", "750"), []int64{250, 750}),

	)

	DescribeTable("invalid splits",
		func(total int64, t split.Type, values []decimal.Decimal, expected error) {
			_, err := split.Compute(total, t, values)
			Expect(err).To(MatchError(expected))
		},
		Entry("unknown type", int64(100), split.Type("foo"), zeros(2), split.ErrUnknownType),
		Entry("zero total", int64(0), split.Equal, zeros(2), split.ErrTotalNotPositive),
		Entry("negative total", int64(-100), split.Equal, zeros(2), split.ErrTotalNotPositive),
		Entry("no participants", int64(100), split.Equal, nil, split.ErrNoParticipants),
		Entry("percentage under 100", int64(100), split.Percentage, ds("50", "49.99"), split.ErrPercentSum),
		Entry("percentage over 100", int64(100), split.Percentage, ds("50", "50.01"), split.ErrPercentSum),
		Entry("percentage negative", int64(100), split.Percentage, ds("150", "-50"), split.ErrNegativeValue),
		Entry("shares all zero", int64(100), split.Shares, ds("0", "0"), split.ErrSharesZero),
		Entry("shares negative", int64(100), split.Shares, ds("-1", "2"), split.ErrNegativeValue),
		Entry("exact under total", int64(1000), split.Exact, ds("250", "700"), split.ErrExactSum),
		Entry("exact over total", int64(1000), split.Exact, ds("250", "800"), split.ErrExactSum),
		Entry("exact single over total", int64(1000), split.Exact, ds("1001", "0"), split.ErrExactSum),
		Entry("exact fractional minor unit", int64(1000), split.Exact, ds("250.5", "749.5"), split.ErrNotWholeUnits),
		Entry("exact negative", int64(1000), split.Exact, ds("-250", "1250"), split.ErrNegativeValue),
	)

	It("reports the actual sum in SumError", func() {
		_, err := split.Compute(100, split.Percentage, ds("50", "40"))
		var se *split.SumError
		Expect(err).To(BeAssignableToTypeOf(se))
		se = err.(*split.SumError)
		Expect(se.Got.String()).To(Equal("90"))
		Expect(se.Want.String()).To(Equal("100"))
	})

	It("is deterministic", func() {
		values := ds("1", "1", "1", "1", "1", "1", "1")
		first, err := split.Compute(1000, split.Shares, values)
		Expect(err).NotTo(HaveOccurred())
		for range 100 {
			Expect(split.Compute(1000, split.Shares, values)).To(Equal(first))
		}
	})

	It("never differs from the exact share by a full unit or more", func() {
		r := rand.New(rand.NewPCG(1, 2))
		for range 2000 {
			n := 1 + r.IntN(8)
			total := 1 + r.Int64N(1_000_000)
			weights := make([]decimal.Decimal, n)
			wsum := decimal.Zero
			for i := range weights {
				weights[i] = decimal.NewFromInt(r.Int64N(1000))
				wsum = wsum.Add(weights[i])
			}
			if wsum.IsZero() {
				continue
			}

			result, err := split.Compute(total, split.Shares, weights)
			Expect(err).NotTo(HaveOccurred())
			Expect(sum(result)).To(Equal(total))

			for i, w := range weights {
				exact := decimal.NewFromInt(total).Mul(w).DivRound(wsum, 10)
				diff := decimal.NewFromInt(result[i]).Sub(exact).Abs()
				Expect(diff.LessThan(decimal.NewFromInt(1))).To(BeTrue(), "share %d: got %d exact %s", i, result[i], exact)
			}
		}
	})
})

func FuzzCompute(f *testing.F) {
	f.Add(int64(100), uint8(0), int64(1), int64(1), int64(1))
	f.Add(int64(10001), uint8(1), int64(3333), int64(3333), int64(3334))
	f.Add(int64(1), uint8(2), int64(1), int64(0), int64(5))
	f.Add(int64(1000), uint8(3), int64(250), int64(250), int64(500))
	f.Add(int64(1000), uint8(4), int64(-10), int64(0), int64(10))

	f.Fuzz(func(t *testing.T, total int64, typ uint8, a, b, c int64) {
		types := split.Types
		st := types[int(typ)%len(types)]
		values := []decimal.Decimal{decimal.NewFromInt(a), decimal.NewFromInt(b), decimal.NewFromInt(c)}
		if st == split.Percentage {
			// Interpret as basis points to reach exactly 100 more often.
			for i := range values {
				values[i] = values[i].Shift(-2)
			}
		}

		result, err := split.Compute(total, st, values)
		if err != nil {
			return
		}

		if len(result) != len(values) {
			t.Fatalf("len mismatch")
		}

		var s int64
		for _, x := range result {
			if x < 0 {
				t.Fatalf("negative share %v for total=%d type=%s values=%v", result, total, st, values)
			}
			s += x
		}
		if s != total {
			t.Fatalf("sum %d != total %d (type=%s values=%v result=%v)", s, total, st, values, result)
		}

		again, err := split.Compute(total, st, values)
		if err != nil || !equal(again, result) {
			t.Fatalf("not deterministic")
		}
	})
}

func equal(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
