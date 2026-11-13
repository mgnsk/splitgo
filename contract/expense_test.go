package contract_test

import (
	"net/url"
	"time"

	"github.com/mgnsk/splitgo/contract"
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/split"
	"github.com/mgnsk/webkit/snowflake"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	alice = &domain.User{ID: 101, Email: "alice@example.com"}
	bob   = &domain.User{ID: 102, Email: "bob@example.com"}
	carol = &domain.User{ID: 103, Email: "carol@example.com"}

	pair = []*domain.User{alice, bob}
	trio = []*domain.User{alice, bob, carol}
)

func baseForm(extra url.Values) url.Values {
	form := url.Values{
		"description": {"Dinner"},
		"amount":      {"30.00"},
		"currency":    {"EUR"},
		"date":        {"2026-11-03"},
	}
	for k, v := range extra {
		form[k] = v
	}
	return form
}

func id(u *domain.User) string { return u.ID.String() }

func validate(me *domain.User, people []*domain.User, form url.Values) *contract.ResolvedExpense {
	GinkgoHelper()
	resolved, errs := contract.ParseExpenseForm(form).Validate(me.ID, people)
	Expect(errs).To(BeEmpty())
	return resolved
}

func validateErrs(me *domain.User, people []*domain.User, form url.Values) url.Values {
	GinkgoHelper()
	resolved, errs := contract.ParseExpenseForm(form).Validate(me.ID, people)
	Expect(resolved).To(BeNil())
	return errs
}

var _ = Describe("ExpenseForm", func() {
	Describe("simple mode with two people", func() {
		DescribeTable("presets",
			func(preset split.Preset, amount string, payer *domain.User, shares map[snowflake.ID]int64) {
				r := validate(alice, pair, baseForm(url.Values{
					"mode":   {"simple"},
					"preset": {string(preset)},
					"amount": {amount},
				}))
				Expect(r.PaidBy).To(Equal(payer.ID))
				Expect(r.SplitType).To(Equal(split.Equal))
				Expect(r.Shares()).To(Equal(shares))
			},
			Entry("A: you paid, split equally", split.YouPaidEqual, "10", alice,
				map[snowflake.ID]int64{alice.ID: 500, bob.ID: 500}),
			Entry("B: you are owed the full amount", split.YouOwedFull, "10", alice,
				map[snowflake.ID]int64{bob.ID: 1000}),
			Entry("C: they paid, split equally", split.TheyPaidEqual, "10", bob,
				map[snowflake.ID]int64{alice.ID: 500, bob.ID: 500}),
			Entry("D: they are owed the full amount", split.TheyOwedFull, "10", bob,
				map[snowflake.ID]int64{alice.ID: 1000}),
			Entry("payer absorbs the odd cent", split.YouPaidEqual, "10.01", alice,
				map[snowflake.ID]int64{alice.ID: 501, bob.ID: 500}),
		)

		It("gives the same result from either person's point of view", func() {
			fromAlice := validate(alice, pair, baseForm(url.Values{"preset": {"you-paid-equal"}, "amount": {"10.01"}}))
			fromBob := validate(bob, pair, baseForm(url.Values{"preset": {"they-paid-equal"}, "amount": {"10.01"}}))
			Expect(fromBob.ResolvedSplit).To(Equal(fromAlice.ResolvedSplit))
		})

		It("requires a valid preset", func() {
			errs := validateErrs(alice, pair, baseForm(url.Values{"preset": {"bogus"}}))
			Expect(errs.Get("preset")).To(Equal("Choose how to split"))
		})
	})

	Describe("simple mode with more people", func() {
		It("splits equally among everyone with the chosen payer absorbing rounding", func() {
			r := validate(alice, trio, baseForm(url.Values{"paid_by": {id(carol)}, "amount": {"100"}}))
			Expect(r.PaidBy).To(Equal(carol.ID))
			Expect(r.Shares()).To(Equal(map[snowflake.ID]int64{alice.ID: 3333, bob.ID: 3333, carol.ID: 3334}))
		})

		It("rejects a payer outside the group", func() {
			errs := validateErrs(alice, trio, baseForm(url.Values{"paid_by": {"999"}}))
			Expect(errs.Get("paid_by")).To(Equal("Choose who paid"))
		})
	})

	Describe("advanced mode", func() {
		advanced := func(typ split.Type, extra url.Values) url.Values {
			form := baseForm(url.Values{"mode": {"advanced"}, "split_type": {string(typ)}, "paid_by": {id(alice)}})
			for k, v := range extra {
				form[k] = v
			}
			return form
		}

		It("doesn't offer equal splits", func() {
			f := contract.ParseExpenseForm(advanced(split.Equal, nil))
			Expect(f.SplitType).To(Equal(split.Percentage))
		})

		It("splits by percentage and stores the inputs", func() {
			r := validate(alice, trio, advanced(split.Percentage, url.Values{
				"value_101": {"50"}, "value_102": {"25,5"}, "value_103": {"24.5"},
			}))
			Expect(r.Participants).To(Equal([]domain.Participant{
				{UserID: alice.ID, Share: 1500, SplitValue: "50"},
				{UserID: bob.ID, Share: 765, SplitValue: "25.5"},
				{UserID: carol.ID, Share: 735, SplitValue: "24.5"},
			}))
		})

		It("explains percentages not adding up", func() {
			errs := validateErrs(alice, trio, advanced(split.Percentage, url.Values{"value_101": {"50"}, "value_102": {"40"}}))
			Expect(errs.Get("split")).To(Equal("Percentages add up to 90%, not 100%"))
		})

		It("splits by shares, omitting people with zero shares", func() {
			r := validate(alice, trio, advanced(split.Shares, url.Values{"value_101": {"1"}, "value_102": {"2"}, "value_103": {""}}))
			Expect(r.Participants).To(Equal([]domain.Participant{
				{UserID: alice.ID, Share: 1000, SplitValue: "1"},
				{UserID: bob.ID, Share: 2000, SplitValue: "2"},
			}))
		})

		It("splits by exact amounts in major units", func() {
			r := validate(alice, trio, advanced(split.Exact, url.Values{"value_101": {"10"}, "value_102": {"12.5"}, "value_103": {"7,50"}}))
			Expect(r.Participants).To(Equal([]domain.Participant{
				{UserID: alice.ID, Share: 1000, SplitValue: "10.00"},
				{UserID: bob.ID, Share: 1250, SplitValue: "12.50"},
				{UserID: carol.ID, Share: 750, SplitValue: "7.50"},
			}))
		})

		It("explains exact amounts not adding up", func() {
			errs := validateErrs(alice, trio, advanced(split.Exact, url.Values{"value_101": {"10"}, "value_102": {"10"}}))
			Expect(errs.Get("split")).To(Equal("Amounts add up to €20.00, €10.00 left to assign"))

			errs = validateErrs(alice, trio, advanced(split.Exact, url.Values{"value_101": {"20"}, "value_102": {"20"}}))
			Expect(errs.Get("split")).To(Equal("Amounts add up to €40.00, €10.00 over the total"))
		})

		It("rejects too precise exact amounts per field", func() {
			errs := validateErrs(alice, trio, advanced(split.Exact, url.Values{"value_101": {"10.001"}}))
			Expect(errs.Get("value_101")).To(Equal("Too many decimal places"))
		})
	})

	Describe("common validation", func() {
		DescribeTable("field errors",
			func(extra url.Values, key, msg string) {
				errs := validateErrs(alice, pair, baseForm(extra))
				Expect(errs.Get(key)).To(Equal(msg))
			},
			Entry("missing description", url.Values{"description": {""}}, "description", "Description is required"),
			Entry("missing amount", url.Values{"amount": {""}}, "amount", "Amount is required"),
			Entry("zero amount", url.Values{"amount": {"0"}}, "amount", "Amount must be greater than zero"),
			Entry("negative amount", url.Values{"amount": {"-5"}}, "amount", "Amount must not be negative"),
			Entry("too precise amount", url.Values{"amount": {"1.001"}}, "amount", "Too many decimal places"),
			Entry("JPY decimals", url.Values{"amount": {"1.5"}, "currency": {"JPY"}}, "amount", "Too many decimal places"),
			Entry("unknown currency", url.Values{"currency": {"XXX"}}, "currency", "Unknown currency"),
			Entry("invalid date", url.Values{"date": {"03.11.2026"}}, "date", "Invalid date"),
		)

		It("rejects users who are not members", func() {
			_, errs := contract.ParseExpenseForm(baseForm(nil)).Validate(carol.ID, pair)
			Expect(errs.Get("split")).To(Equal("You are not a member of this group"))
		})
	})

	Describe("editing", func() {
		expenseFrom := func(me *domain.User, people []*domain.User, form url.Values) *domain.Expense {
			r := validate(me, people, form)
			return &domain.Expense{
				Description:  r.Description,
				Amount:       r.Amount,
				Currency:     r.Currency.Code,
				SplitType:    r.SplitType,
				PaidBy:       r.PaidBy,
				Date:         r.Date,
				Participants: r.Participants,
			}
		}

		It("reopens two-person preset expenses in simple mode from both sides", func() {
			for _, preset := range split.Presets {
				e := expenseFrom(alice, pair, baseForm(url.Values{"preset": {string(preset)}}))

				f := contract.ExpenseFormFromExpense(e, alice.ID, pair)
				Expect(f.Mode).To(Equal(contract.ModeSimple))
				Expect(f.Preset).To(Equal(preset))
				Expect(f.Amount).To(Equal("30.00"))

				// Bob sees the mirrored preset.
				fb := contract.ExpenseFormFromExpense(e, bob.ID, pair)
				Expect(fb.Mode).To(Equal(contract.ModeSimple))
				r := validate(bob, pair, formValues(fb))
				Expect(r.ResolvedSplit).To(Equal(contract.ResolvedSplit{PaidBy: e.PaidBy, SplitType: e.SplitType, Participants: e.Participants}))
			}
		})

		It("reopens equal-among-all expenses in simple mode", func() {
			e := expenseFrom(alice, trio, baseForm(url.Values{"paid_by": {id(bob)}}))
			f := contract.ExpenseFormFromExpense(e, alice.ID, trio)
			Expect(f.Mode).To(Equal(contract.ModeSimple))
			Expect(f.PaidBy).To(Equal(bob.ID))
		})

		It("reopens other splits in advanced mode with the original inputs", func() {
			e := expenseFrom(alice, trio, baseForm(url.Values{
				"mode": {"advanced"}, "split_type": {"percentage"}, "paid_by": {id(alice)},
				"value_101": {"33.33"}, "value_102": {"33.33"}, "value_103": {"33.34"},
			}))
			f := contract.ExpenseFormFromExpense(e, alice.ID, trio)
			Expect(f.Mode).To(Equal(contract.ModeAdvanced))
			Expect(f.SplitType).To(Equal(split.Percentage))
			Expect(f.Values).To(HaveKeyWithValue(carol.ID, "33.34"))

			// Re-submitting unchanged yields the same split.
			r := validate(alice, trio, formValues(f))
			Expect(r.Participants).To(Equal(e.Participants))
		})

		It("reopens equal splits among some people as one share each", func() {
			e := &domain.Expense{
				Description: "Lunch", Amount: 3000, Currency: "EUR", SplitType: split.Equal, PaidBy: alice.ID,
				Participants: []domain.Participant{{UserID: bob.ID, Share: 1500}, {UserID: carol.ID, Share: 1500}},
			}
			f := contract.ExpenseFormFromExpense(e, alice.ID, trio)
			Expect(f.Mode).To(Equal(contract.ModeAdvanced))
			Expect(f.SplitType).To(Equal(split.Shares))
			Expect(f.Values).To(Equal(map[snowflake.ID]string{bob.ID: "1", carol.ID: "1"}))

			r := validate(alice, trio, formValues(f))
			Expect(r.Shares()).To(Equal(map[snowflake.ID]int64{bob.ID: 1500, carol.ID: 1500}))
		})
	})

	Describe("switching to advanced mode", func() {
		It("carries over the preset's payer and participants", func() {
			f := contract.ParseExpenseForm(baseForm(url.Values{"preset": {"they-owed-full"}}))
			f.SwitchToAdvanced(alice.ID, pair)
			Expect(f.Mode).To(Equal(contract.ModeAdvanced))
			Expect(f.PaidBy).To(Equal(bob.ID))
			Expect(f.SplitType).To(Equal(split.Percentage))
			Expect(f.Values).To(Equal(map[snowflake.ID]string{alice.ID: "100", bob.ID: "0"}))

			r := validate(alice, pair, formValues(f))
			Expect(r.Shares()).To(Equal(map[snowflake.ID]int64{alice.ID: 3000}))
		})

		It("carries over an equal preset as 50/50", func() {
			f := contract.ParseExpenseForm(baseForm(url.Values{"preset": {"you-paid-equal"}}))
			f.SwitchToAdvanced(alice.ID, pair)
			Expect(f.Values).To(Equal(map[snowflake.ID]string{alice.ID: "50", bob.ID: "50"}))
		})

		It("gives everyone one share in larger groups", func() {
			f := contract.ParseExpenseForm(baseForm(url.Values{"paid_by": {id(carol)}}))
			f.SwitchToAdvanced(alice.ID, trio)
			Expect(f.PaidBy).To(Equal(carol.ID))
			Expect(f.SplitType).To(Equal(split.Shares))
			Expect(f.Values).To(Equal(map[snowflake.ID]string{alice.ID: "1", bob.ID: "1", carol.ID: "1"}))
		})
	})

	It("NewExpenseForm defaults to simple mode and today", func() {
		g := &domain.Group{Currency: "USD"}
		f := contract.NewExpenseForm(g, alice.ID, time.Date(2026, 11, 3, 15, 0, 0, 0, time.UTC))
		Expect(f.Mode).To(Equal(contract.ModeSimple))
		Expect(f.Preset).To(Equal(split.YouPaidEqual))
		Expect(f.Currency).To(Equal("USD"))
		Expect(f.Date).To(Equal("2026-11-03"))
	})
})

// formValues serializes a form back to request values, like the browser does.
func formValues(f *contract.ExpenseForm) url.Values {
	v := url.Values{
		"description": {f.Description},
		"amount":      {f.Amount},
		"currency":    {f.Currency},
		"date":        {f.Date},
		"mode":        {f.Mode},
		"preset":      {string(f.Preset)},
		"paid_by":     {f.PaidBy.String()},
		"split_type":  {string(f.SplitType)},
	}
	for uid, s := range f.Values {
		v.Set(contract.ValueKey(uid), s)
	}
	return v
}
