package mail_test

import (
	"github.com/mgnsk/splitgo/mail"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = DescribeTable("ParseFrom",
	func(from, wantAddr, wantHeader string) {
		addr, err := mail.ParseFrom(from)
		Expect(err).NotTo(HaveOccurred())
		Expect(addr.Address).To(Equal(wantAddr))
		Expect(addr.String()).To(Equal(wantHeader))
	},
	Entry("bare address", "noreply@example.com", "noreply@example.com", "<noreply@example.com>"),
	Entry("angle brackets", "<noreply@example.com>", "noreply@example.com", "<noreply@example.com>"),
	Entry("display name", "splitgo <noreply@example.com>", "noreply@example.com", `"splitgo" <noreply@example.com>`),
	Entry("quoted display name", `"Split Go" <noreply@example.com>`, "noreply@example.com", `"Split Go" <noreply@example.com>`),
	Entry("surrounding spaces", "  noreply@example.com ", "noreply@example.com", "<noreply@example.com>"),
)

var _ = DescribeTable("ParseFrom rejects",
	func(from string) {
		_, err := mail.ParseFrom(from)
		Expect(err).To(HaveOccurred())
	},
	Entry("empty", ""),
	Entry("no domain", "noreply"),
	Entry("unclosed bracket", "splitgo <noreply@example.com"),
)

var _ = Describe("NewSMTPSender", func() {
	It("rejects an invalid sender address", func() {
		_, err := mail.NewSMTPSender("smtp://host:587", "not an address")
		Expect(err).To(HaveOccurred())
	})

	It("accepts a sender with a display name", func() {
		_, err := mail.NewSMTPSender("smtp://user:pass@host:587", "splitgo <noreply@example.com>")
		Expect(err).NotTo(HaveOccurred())
	})
})
