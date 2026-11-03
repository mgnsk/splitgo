package model_test

import (
	"time"

	"github.com/mgnsk/splitgo"
	"github.com/mgnsk/splitgo/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("login tokens", func() {
	now := time.Unix(1_800_000_000, 0)

	It("can be consumed exactly once", func(ctx SpecContext) {
		Expect(model.InsertLoginToken(ctx, db, "hash", "A@example.com", now, 15*time.Minute)).To(Succeed())

		Expect(model.GetLoginToken(ctx, db, "hash", now)).To(Equal("a@example.com"))
		Expect(model.ConsumeLoginToken(ctx, db, "hash", now)).To(Equal("a@example.com"))

		_, err := model.ConsumeLoginToken(ctx, db, "hash", now)
		Expect(err).To(MatchError(splitgo.NotFound))
	})

	It("can not be consumed after expiry", func(ctx SpecContext) {
		Expect(model.InsertLoginToken(ctx, db, "hash", "a@example.com", now, 15*time.Minute)).To(Succeed())

		_, err := model.ConsumeLoginToken(ctx, db, "hash", now.Add(15*time.Minute))
		Expect(err).To(MatchError(splitgo.NotFound))
	})

	It("tracks the last token time and deletes expired tokens", func(ctx SpecContext) {
		Expect(model.LastLoginTokenTime(ctx, db, "a@example.com")).To(BeZero())

		Expect(model.InsertLoginToken(ctx, db, "h1", "a@example.com", now, time.Minute)).To(Succeed())
		Expect(model.InsertLoginToken(ctx, db, "h2", "a@example.com", now.Add(time.Hour), time.Minute)).To(Succeed())
		Expect(model.LastLoginTokenTime(ctx, db, "a@example.com")).To(Equal(now.Add(time.Hour)))

		Expect(model.DeleteExpiredLoginTokens(ctx, db, now.Add(30*time.Minute))).To(Succeed())
		_, err := model.GetLoginToken(ctx, db, "h1", now)
		Expect(err).To(MatchError(splitgo.NotFound))
		Expect(model.GetLoginToken(ctx, db, "h2", now.Add(time.Hour))).To(Equal("a@example.com"))
	})
})
