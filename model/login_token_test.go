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
		Expect(model.InsertLoginToken(ctx, db, "hash", "", "A@example.com", now, 15*time.Minute)).To(Succeed())

		Expect(model.GetLoginToken(ctx, db, "hash", now)).To(Equal("a@example.com"))
		Expect(model.ConsumeLoginToken(ctx, db, "hash", now)).To(Equal("a@example.com"))

		_, err := model.ConsumeLoginToken(ctx, db, "hash", now)
		Expect(err).To(MatchError(splitgo.NotFound))
	})

	It("can not be consumed after expiry", func(ctx SpecContext) {
		Expect(model.InsertLoginToken(ctx, db, "hash", "", "a@example.com", now, 15*time.Minute)).To(Succeed())

		_, err := model.ConsumeLoginToken(ctx, db, "hash", now.Add(15*time.Minute))
		Expect(err).To(MatchError(splitgo.NotFound))
	})

	It("tracks the last token time and deletes expired tokens", func(ctx SpecContext) {
		Expect(model.LastLoginTokenTime(ctx, db, "a@example.com")).To(BeZero())

		Expect(model.InsertLoginToken(ctx, db, "h1", "", "a@example.com", now, time.Minute)).To(Succeed())
		Expect(model.InsertLoginToken(ctx, db, "h2", "", "a@example.com", now.Add(time.Hour), time.Minute)).To(Succeed())
		Expect(model.LastLoginTokenTime(ctx, db, "a@example.com")).To(Equal(now.Add(time.Hour)))

		Expect(model.DeleteExpiredLoginTokens(ctx, db, now.Add(30*time.Minute))).To(Succeed())
		_, err := model.GetLoginToken(ctx, db, "h1", now)
		Expect(err).To(MatchError(splitgo.NotFound))
		Expect(model.GetLoginToken(ctx, db, "h2", now.Add(time.Hour))).To(Equal("a@example.com"))
	})
})

var _ = Describe("login codes", func() {
	now := time.Unix(1_800_000_000, 0)

	hashFor := func(code string) func(string) string {
		return func(tokenHash string) string { return tokenHash + ":" + code }
	}

	insert := func(ctx SpecContext, tokenHash, code string, at time.Time) {
		Expect(model.InsertLoginToken(ctx, db, tokenHash, tokenHash+":"+code, "a@example.com", at, 15*time.Minute)).To(Succeed())
	}

	It("consumes the token with the right code", func(ctx SpecContext) {
		insert(ctx, "t1", "123456", now)

		Expect(model.ConsumeLoginCode(ctx, db, "A@example.com", hashFor("123456"), now)).To(Succeed())

		_, err := model.GetLoginToken(ctx, db, "t1", now)
		Expect(err).To(MatchError(splitgo.NotFound))
		Expect(model.ConsumeLoginCode(ctx, db, "a@example.com", hashFor("123456"), now)).To(MatchError(splitgo.NotFound))
	})

	It("checks the newest token", func(ctx SpecContext) {
		insert(ctx, "old", "111111", now)
		insert(ctx, "new", "222222", now.Add(time.Minute))

		Expect(model.ConsumeLoginCode(ctx, db, "a@example.com", hashFor("111111"), now.Add(time.Minute))).To(MatchError(splitgo.InvalidValue))
		Expect(model.ConsumeLoginCode(ctx, db, "a@example.com", hashFor("222222"), now.Add(time.Minute))).To(Succeed())
	})

	It("counts wrong codes and disables the code after too many attempts", func(ctx SpecContext) {
		insert(ctx, "t1", "123456", now)

		for range model.MaxCodeAttempts {
			Expect(model.ConsumeLoginCode(ctx, db, "a@example.com", hashFor("000000"), now)).To(MatchError(splitgo.InvalidValue))
		}
		Expect(model.CountLoginFailures(ctx, db, "a@example.com", now.Add(-time.Hour))).To(Equal(model.MaxCodeAttempts))

		// Even the right code no longer works, but the link does.
		Expect(model.ConsumeLoginCode(ctx, db, "a@example.com", hashFor("123456"), now)).To(MatchError(splitgo.NotFound))
		Expect(model.ConsumeLoginToken(ctx, db, "t1", now)).To(Equal("a@example.com"))
	})

	It("ignores tokens without a code and expired tokens", func(ctx SpecContext) {
		Expect(model.InsertLoginToken(ctx, db, "invite", "", "a@example.com", now, time.Hour)).To(Succeed())
		Expect(model.ConsumeLoginCode(ctx, db, "a@example.com", hashFor(""), now)).To(MatchError(splitgo.NotFound))

		insert(ctx, "t1", "123456", now)
		Expect(model.ConsumeLoginCode(ctx, db, "a@example.com", hashFor("123456"), now.Add(15*time.Minute))).To(MatchError(splitgo.NotFound))
	})

	It("forgets old failures", func(ctx SpecContext) {
		insert(ctx, "t1", "123456", now)
		Expect(model.ConsumeLoginCode(ctx, db, "a@example.com", hashFor("000000"), now)).To(MatchError(splitgo.InvalidValue))

		Expect(model.DeleteExpiredLoginTokens(ctx, db, now.Add(model.LoginFailureWindow))).To(Succeed())
		Expect(model.CountLoginFailures(ctx, db, "a@example.com", now.Add(-time.Hour))).To(Equal(0))
	})
})
