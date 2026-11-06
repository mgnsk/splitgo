package model_test

import (
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/model"
	"github.com/mgnsk/webkit"
	. "github.com/mgnsk/webkit/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("friends", func() {
	It("creates one direct group per pair", func(ctx SpecContext) {
		alice := mustCreateUser(ctx, "alice@example.com")
		bob := mustCreateUser(ctx, "bob@example.com")

		g, created, err := model.GetOrCreateDirectGroup(ctx, db, alice.ID, bob.ID, "EUR")
		Expect(err).NotTo(HaveOccurred())
		Expect(created).To(BeTrue())
		Expect(g.Direct()).To(BeTrue())
		Expect(Must(model.ListGroupMembers(ctx, db, g.ID))).To(HaveLen(2))

		again, created, err := model.GetOrCreateDirectGroup(ctx, db, bob.ID, alice.ID, "USD")
		Expect(err).NotTo(HaveOccurred())
		Expect(created).To(BeFalse())
		Expect(again.ID).To(Equal(g.ID))
		Expect(again.Currency).To(Equal("EUR"))

		_, _, err = model.GetOrCreateDirectGroup(ctx, db, alice.ID, alice.ID, "EUR")
		Expect(err).To(MatchError(webkit.InvalidValue))
	})

	It("lists friends but not group members", func(ctx SpecContext) {
		alice := mustCreateUser(ctx, "alice@example.com")
		bob := mustCreateUser(ctx, "bob@example.com")
		carol := mustCreateUser(ctx, "carol@example.com")
		mustCreateGroup(ctx, "Trip", alice, carol)

		g, _ := Must2(model.GetOrCreateDirectGroup(ctx, db, alice.ID, bob.ID, "EUR"))

		Expect(Must(model.ListFriends(ctx, db, alice.ID))).To(HaveExactElements(
			And(HaveField("User.ID", bob.ID), HaveField("DirectGroupID", g.ID)),
		))
		Expect(Must(model.ListFriends(ctx, db, bob.ID))).To(HaveExactElements(HaveField("User.ID", alice.ID)))
		Expect(Must(model.ListFriends(ctx, db, carol.ID))).To(BeEmpty())
	})

	It("does not let anyone join a direct group by invite code", func(ctx SpecContext) {
		alice := mustCreateUser(ctx, "alice@example.com")
		bob := mustCreateUser(ctx, "bob@example.com")
		g, _ := Must2(model.GetOrCreateDirectGroup(ctx, db, alice.ID, bob.ID, "EUR"))

		_, err := model.GetGroupByInviteCode(ctx, db, g.InviteCode)
		Expect(err).To(MatchError(webkit.NotFound))
	})

	It("keeps ordinary groups ordinary", func(ctx SpecContext) {
		alice := mustCreateUser(ctx, "alice@example.com")
		g := mustCreateGroup(ctx, "Trip", alice)
		Expect(Must(model.GetGroup(ctx, db, g.ID)).Kind).To(Equal(domain.GroupKindGroup))
	})
})
