package model_test

import (
	"github.com/mgnsk/splitgo"
	"github.com/mgnsk/splitgo/model"
	. "github.com/mgnsk/webkit/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("groups", func() {
	It("adds the creator as a member", func(ctx SpecContext) {
		alice := mustCreateUser(ctx, "alice@example.com")
		g := mustCreateGroup(ctx, "Trip", alice)

		Expect(model.IsGroupMember(ctx, db, g.ID, alice.ID)).To(BeTrue())
		Expect(Must(model.ListGroupMembers(ctx, db, g.ID))).To(HaveExactElements(HaveField("ID", alice.ID)))
	})

	It("lists only the user's groups", func(ctx SpecContext) {
		alice := mustCreateUser(ctx, "alice@example.com")
		bob := mustCreateUser(ctx, "bob@example.com")
		mustCreateGroup(ctx, "b-group", alice, bob)
		mustCreateGroup(ctx, "A-group", alice)
		mustCreateGroup(ctx, "bobs", bob)

		Expect(Must(model.ListUserGroups(ctx, db, alice.ID))).To(HaveExactElements(
			HaveField("Name", "A-group"),
			HaveField("Name", "b-group"),
		))
	})

	It("restricts access to members", func(ctx SpecContext) {
		alice := mustCreateUser(ctx, "alice@example.com")
		mallory := mustCreateUser(ctx, "mallory@example.com")
		g := mustCreateGroup(ctx, "Trip", alice)

		Expect(Must(model.GetUserGroup(ctx, db, alice.ID, g.ID)).Name).To(Equal("Trip"))
		_, err := model.GetUserGroup(ctx, db, mallory.ID, g.ID)
		Expect(err).To(MatchError(splitgo.NotFound))
	})

	It("adds members idempotently and removes them", func(ctx SpecContext) {
		alice := mustCreateUser(ctx, "alice@example.com")
		bob := mustCreateUser(ctx, "bob@example.com")
		g := mustCreateGroup(ctx, "Trip", alice)

		Expect(model.AddGroupMember(ctx, db, g.ID, bob.ID)).To(Succeed())
		Expect(model.AddGroupMember(ctx, db, g.ID, bob.ID)).To(Succeed())
		Expect(Must(model.ListGroupMembers(ctx, db, g.ID))).To(HaveLen(2))

		Expect(model.RemoveGroupMember(ctx, db, g.ID, bob.ID)).To(Succeed())
		Expect(model.IsGroupMember(ctx, db, g.ID, bob.ID)).To(BeFalse())
	})

	It("finds groups by invite code and updates settings", func(ctx SpecContext) {
		alice := mustCreateUser(ctx, "alice@example.com")
		g := mustCreateGroup(ctx, "Trip", alice)

		Expect(Must(model.GetGroupByInviteCode(ctx, db, g.InviteCode)).ID).To(Equal(g.ID))

		g.Name = "Renamed"
		g.SimplifyDebts = true
		g.Currency = "USD"
		Expect(model.UpdateGroup(ctx, db, g)).To(Succeed())
		Expect(Must(model.GetGroup(ctx, db, g.ID))).To(Equal(g))
	})

	It("deletes a group with its expenses", func(ctx SpecContext) {
		alice := mustCreateUser(ctx, "alice@example.com")
		bob := mustCreateUser(ctx, "bob@example.com")
		g := mustCreateGroup(ctx, "Trip", alice, bob)
		mustAddEqualExpense(ctx, g, alice, 1000, alice, bob)

		Expect(model.DeleteGroup(ctx, db, g.ID)).To(Succeed())
		Expect(Must(model.ListUserExpenses(ctx, db, alice.ID, nil, 10))).To(BeEmpty())
		_, err := model.GetGroup(ctx, db, g.ID)
		Expect(err).To(MatchError(splitgo.NotFound))
	})
})
