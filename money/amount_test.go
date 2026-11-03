package money_test

import (
	"testing"

	"github.com/mgnsk/splitgo/money"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/shopspring/decimal"
)

var (
	eur = money.MustCurrency("EUR")
	jpy = money.MustCurrency("JPY")
	kwd = money.MustCurrency("KWD")
	sek = money.MustCurrency("SEK")
)

var _ = Describe("ParseMinor", func() {
	DescribeTable("valid input",
		func(input string, c money.Currency, expected int64) {
			Expect(money.ParseMinor(input, c)).To(Equal(expected))
		},
		Entry("integer", "12", eur, int64(1200)),
		Entry("one decimal", "12.5", eur, int64(1250)),
		Entry("two decimals", "12.50", eur, int64(1250)),
		Entry("comma decimal", "12,50", eur, int64(1250)),
		Entry("leading dot", ".5", eur, int64(50)),
		Entry("one cent", "0.01", eur, int64(1)),
		Entry("surrounding whitespace", "  7.25 ", eur, int64(725)),
		Entry("space grouping", "1 234,56", eur, int64(123456)),
		Entry("nbsp grouping", "1 234,56", eur, int64(123456)),
		Entry("US grouping", "1,234.56", eur, int64(123456)),
		Entry("EU grouping", "1.234,56", eur, int64(123456)),
		Entry("multiple dot groups", "1.234.567", eur, int64(123456700)),
		Entry("multiple comma groups", "1,234,567", eur, int64(123456700)),
		Entry("trailing zeros beyond precision", "12.500", eur, int64(1250)),
		Entry("JPY integer", "1200", jpy, int64(1200)),
		Entry("JPY trailing zero decimals", "1200.00", jpy, int64(1200)),
		Entry("KWD three decimals", "1.234", kwd, int64(1234)),
		Entry("KWD comma three decimals", "1,234", kwd, int64(1234)),
		Entry("max amount", "1000000000000.00", eur, money.MaxMinor),
	)

	DescribeTable("invalid input",
		func(input string, c money.Currency, expected error) {
			_, err := money.ParseMinor(input, c)
			Expect(err).To(MatchError(expected))
		},
		Entry("empty", "", eur, money.ErrEmpty),
		Entry("blank", "   ", eur, money.ErrEmpty),
		Entry("negative", "-5", eur, money.ErrNegative),
		Entry("zero", "0", eur, money.ErrNotPositive),
		Entry("zero decimals", "0.00", eur, money.ErrNotPositive),
		Entry("too precise EUR", "0.001", eur, money.ErrTooPrecise),
		Entry("single comma is decimal sep (ambiguous 1,234 for EUR)", "1,234", eur, money.ErrTooPrecise),
		Entry("too precise JPY", "12.5", jpy, money.ErrTooPrecise),
		Entry("too large", "1000000000000.01", eur, money.ErrTooLarge),
		Entry("huge", "99999999999999999999999", eur, money.ErrTooLarge),
		Entry("letters", "12a", eur, money.ErrInvalid),
		Entry("currency symbol", "€12", eur, money.ErrInvalid),
		Entry("exponent", "1e3", eur, money.ErrInvalid),
		Entry("plus sign", "+5", eur, money.ErrInvalid),
		Entry("only dot", ".", eur, money.ErrInvalid),
		Entry("double decimal", "1.2,3.4", eur, money.ErrInvalid),
		Entry("NaN", "NaN", eur, money.ErrInvalid),
		Entry("Inf", "Inf", eur, money.ErrInvalid),
	)
})

var _ = Describe("ToMinor and FromMinor", func() {
	It("round-trips", func() {
		for _, c := range money.Currencies() {
			for _, minor := range []int64{0, 1, 5, 99, 100, 101, 123456789, money.MaxMinor} {
				Expect(money.ToMinor(money.FromMinor(minor, c), c)).To(Equal(minor), c.Code)
			}
		}
	})

	It("rejects fractional minor units", func() {
		_, err := money.ToMinor(decimal.RequireFromString("1.005"), eur)
		Expect(err).To(MatchError(money.ErrTooPrecise))
	})
})

var _ = Describe("formatting", func() {
	DescribeTable("Format",
		func(minor int64, c money.Currency, expected string) {
			Expect(money.Format(minor, c)).To(Equal(expected))
		},
		Entry("zero", int64(0), eur, "€0.00"),
		Entry("one cent", int64(1), eur, "€0.01"),
		Entry("cents", int64(1250), eur, "€12.50"),
		Entry("grouping", int64(123456789), eur, "€1,234,567.89"),
		Entry("exact thousand", int64(100000), eur, "€1,000.00"),
		Entry("negative", int64(-1250), eur, "-€12.50"),
		Entry("JPY", int64(1200), jpy, "¥1,200"),
		Entry("KWD uses code", int64(1234), kwd, "1.234 KWD"),
		Entry("SEK uses code", int64(9900), sek, "99.00 SEK"),
	)

	DescribeTable("FormatPlain",
		func(minor int64, c money.Currency, expected string) {
			Expect(money.FormatPlain(minor, c)).To(Equal(expected))
		},
		Entry("EUR", int64(1250), eur, "12.50"),
		Entry("EUR grouping free", int64(123456789), eur, "1234567.89"),
		Entry("JPY", int64(1200), jpy, "1200"),
		Entry("KWD", int64(5), kwd, "0.005"),
	)

	It("FormatPlain output parses back to the same amount", func() {
		for _, c := range money.Currencies() {
			for _, minor := range []int64{1, 7, 10, 999, 100000, money.MaxMinor} {
				Expect(money.ParseMinor(money.FormatPlain(minor, c), c)).To(Equal(minor), c.Code)
			}
		}
	})
})

var _ = Describe("currencies", func() {
	It("has sane decimals and sorted codes", func() {
		list := money.Currencies()
		Expect(list).NotTo(BeEmpty())
		for i, c := range list {
			Expect(c.Code).To(HaveLen(3))
			Expect(c.Decimals).To(BeNumerically(">=", 0))
			Expect(c.Decimals).To(BeNumerically("<=", 3))
			if i > 0 {
				Expect(c.Code > list[i-1].Code).To(BeTrue())
			}
		}
	})

	It("rejects unknown codes", func() {
		_, ok := money.LookupCurrency("XXX")
		Expect(ok).To(BeFalse())
	})
})

func FuzzParseMinor(f *testing.F) {
	for _, s := range []string{"12", "12.50", "1,234.56", "1.234,56", "0.01", "-1", "1e5", "", " 1 000 "} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		for _, c := range []money.Currency{eur, jpy, kwd} {
			minor, err := money.ParseMinor(s, c)
			if err != nil {
				continue
			}
			if minor <= 0 || minor > money.MaxMinor {
				t.Fatalf("ParseMinor(%q, %s) = %d out of range", s, c.Code, minor)
			}
			again, err := money.ParseMinor(money.FormatPlain(minor, c), c)
			if err != nil || again != minor {
				t.Fatalf("round-trip of %q (%s): %d -> %d, %v", s, c.Code, minor, again, err)
			}
		}
	})
}
