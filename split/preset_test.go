package split_test

import (
	"github.com/mgnsk/splitgo/split"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/shopspring/decimal"
)

const (
	me    int64 = 1
	other int64 = 2
	third int64 = 3
)

// presetShares returns the payer and each participant's share for a preset
// expense, computed the same way as the application does.
func presetShares(p split.Preset, total int64) (payer int64, shares map[int64]int64) {
	plan, ok := split.Resolve(p, me, other)
	Expect(ok).To(BeTrue())

	result, err := split.Compute(total, split.Equal, make([]decimal.Decimal, len(plan.Participants)))
	Expect(err).NotTo(HaveOccurred())

	shares = map[int64]int64{}
	for i, id := range plan.Participants {
		shares[id] = result[i]
	}
	return plan.Payer, shares
}

var _ = Describe("presets", func() {
	DescribeTable("resolve to the expected payer and debt",
		func(p split.Preset, total int64, expectedPayer, debtor, expectedDebt int64) {
			payer, shares := presetShares(p, total)
			Expect(payer).To(Equal(expectedPayer))
			Expect(shares[debtor]).To(Equal(expectedDebt))

			var s int64
			for _, v := range shares {
				s += v
			}
			Expect(s).To(Equal(total))
		},
		Entry("A: you paid, split equally", split.YouPaidEqual, int64(1000), me, other, int64(500)),
		Entry("B: you are owed the full amount", split.YouOwedFull, int64(1000), me, other, int64(1000)),
		Entry("C: they paid, split equally", split.TheyPaidEqual, int64(1000), other, me, int64(500)),
		Entry("D: they are owed the full amount", split.TheyOwedFull, int64(1000), other, me, int64(1000)),
		Entry("A with odd cent: payer (first in order) absorbs the extra cent", split.YouPaidEqual, int64(1001), me, other, int64(500)),
		Entry("C with odd cent: debtor (first in order) pays the extra cent", split.TheyPaidEqual, int64(1001), other, me, int64(501)),
		Entry("B with odd cent", split.YouOwedFull, int64(1001), me, other, int64(1001)),
	)

	It("rejects unknown presets", func() {
		_, ok := split.Resolve(split.Preset("nope"), me, other)
		Expect(ok).To(BeFalse())
		Expect(split.Preset("nope").Valid()).To(BeFalse())
	})

	It("round-trips every preset through detection", func() {
		for _, p := range split.Presets {
			plan, ok := split.Resolve(p, me, other)
			Expect(ok).To(BeTrue())
			Expect(mustDetect(split.Equal, plan.Payer, plan.Participants, me, other)).To(Equal(p), string(p))

			// Order of participants does not matter.
			reversed := []int64{}
			for i := len(plan.Participants) - 1; i >= 0; i-- {
				reversed = append(reversed, plan.Participants[i])
			}
			Expect(mustDetect(split.Equal, plan.Payer, reversed, me, other)).To(Equal(p), string(p))
		}
	})

	It("detects from the other person's point of view", func() {
		plan, _ := split.Resolve(split.YouPaidEqual, me, other)
		Expect(mustDetect(split.Equal, plan.Payer, plan.Participants, other, me)).To(Equal(split.TheyPaidEqual))

		plan, _ = split.Resolve(split.YouOwedFull, me, other)
		Expect(mustDetect(split.Equal, plan.Payer, plan.Participants, other, me)).To(Equal(split.TheyOwedFull))
	})

	DescribeTable("non-preset splits are not detected",
		func(t split.Type, payer int64, participants []int64) {
			_, ok := split.Detect(t, payer, participants, me, other)
			Expect(ok).To(BeFalse())
		},
		Entry("non-equal type", split.Percentage, me, []int64{me, other}),
		Entry("shares type", split.Shares, other, []int64{me, other}),
		Entry("third participant", split.Equal, me, []int64{me, other, third}),
		Entry("third person paid", split.Equal, third, []int64{me, other}),
		Entry("only payer participates", split.Equal, me, []int64{me}),
		Entry("no participants", split.Equal, me, []int64{}),
	)

	It("detects equal among all members", func() {
		members := []int64{me, other, third}
		Expect(split.IsEqualAmongAll(split.Equal, []int64{third, me, other}, members)).To(BeTrue())
		Expect(split.IsEqualAmongAll(split.Equal, []int64{me, other}, members)).To(BeFalse())
		Expect(split.IsEqualAmongAll(split.Shares, []int64{me, other, third}, members)).To(BeFalse())
	})
})

func mustDetect(t split.Type, payer int64, participants []int64, me, other int64) split.Preset {
	GinkgoHelper()
	p, ok := split.Detect(t, payer, participants, me, other)
	Expect(ok).To(BeTrue())
	return p
}
