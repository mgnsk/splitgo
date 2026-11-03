package contract_test

import (
	"github.com/mgnsk/splitgo/contract"
	"github.com/mgnsk/splitgo/pkg/snowflake"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SettleForm", func() {
	members := []snowflake.ID{alice.ID, bob.ID}

	It("validates a settlement", func() {
		f := contract.SettleForm{From: alice.ID, To: bob.ID, Amount: "12,34", Currency: "EUR", Date: "2026-11-03"}
		r, errs := f.Validate(members)
		Expect(errs).To(BeEmpty())
		Expect(r.Amount).To(Equal(int64(1234)))
	})

	DescribeTable("errors",
		func(f contract.SettleForm, key, msg string) {
			_, errs := f.Validate(members)
			Expect(errs.Get(key)).To(Equal(msg))
		},
		Entry("same person", contract.SettleForm{From: alice.ID, To: alice.ID, Amount: "1", Currency: "EUR", Date: "2026-11-03"}, "to", "Payer and recipient must be different people"),
		Entry("non-member", contract.SettleForm{From: carol.ID, To: alice.ID, Amount: "1", Currency: "EUR", Date: "2026-11-03"}, "from", "Choose who paid"),
		Entry("zero amount", contract.SettleForm{From: alice.ID, To: bob.ID, Amount: "0", Currency: "EUR", Date: "2026-11-03"}, "amount", "Amount must be greater than zero"),
		Entry("bad date", contract.SettleForm{From: alice.ID, To: bob.ID, Amount: "1", Currency: "EUR", Date: ""}, "date", "Invalid date"),
	)
})

var _ = Describe("simple forms", func() {
	DescribeTable("email validation",
		func(email, msg string) {
			f := contract.LoginForm{Email: email}
			Expect(f.Validate().Get("email")).To(Equal(msg))
		},
		Entry("valid", "a@example.com", ""),
		Entry("trimmed", "  a@example.com ", ""),
		Entry("empty", "", "Email is required"),
		Entry("no at", "example.com", "Invalid email address"),
		Entry("display name", "A <a@example.com>", "Invalid email address"),
		Entry("header injection", "a@example.com\r\nBcc: x@example.com", "Invalid email address"),
	)

	It("validates groups", func() {
		f := contract.GroupForm{Name: "  ", Currency: "EUR"}
		Expect(f.Validate().Get("name")).To(Equal("Name is required"))

		f = contract.GroupForm{Name: "Trip", Currency: "XYZ"}
		Expect(f.Validate().Get("currency")).To(Equal("Unknown currency"))
	})
})
