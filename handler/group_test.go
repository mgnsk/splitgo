package handler_test

import (
	"net/http"
	"net/url"
	"regexp"

	"github.com/mgnsk/splitgo/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("groups", func() {
	var (
		env          *testEnv
		alice, bob   *client
		aliceU, bobU *domain.User
		group        string
	)

	BeforeEach(func() {
		env = newTestEnv(false)
		alice = env.newClient()
		aliceU = alice.login("alice@example.com")
		group = alice.createGroup("Trip")
	})

	addBob := func() {
		GinkgoHelper()
		resp := alice.post(group+"/members", url.Values{"email": {"bob@example.com"}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))

		// Bob signs in via the invitation link and lands in the group.
		bob = env.newClient()
		link := env.mailer.lastLinkTo("bob@example.com")
		resp = bob.post(link, nil, false)
		Expect(resp.Header.Get("Location")).To(Equal(group))
		bobU = &domain.User{}
		Expect(db.NewSelect().Table("users").Where("email = ?", "bob@example.com").Column("id").Scan(GinkgoT().Context(), &bobU.ID)).To(Succeed())
	}

	It("shows the group on the home page", func() {
		Expect(text(alice.get("/").body)).To(ContainSubstring("Trip settled up"))
		Expect(alice.get(group).StatusCode).To(Equal(http.StatusOK))
	})

	It("validates the group form", func() {
		resp := alice.post("/groups", url.Values{"name": {""}, "currency": {"EUR"}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.body).To(ContainSubstring("Name is required"))
	})

	It("adds members by email and invites them", func() {
		addBob()

		msgs := env.mailer.messages()
		Expect(msgs[len(msgs)-1].Subject).To(Equal("alice added you to Trip on splitgo"))
		Expect(text(bob.get(group).body)).To(ContainSubstring("2 members"))
	})

	It("suggests friends who aren't members when adding by email", func(ctx SpecContext) {
		mustCreateUser(ctx, "carol@example.com")
		for _, email := range []string{"bob@example.com", "carol@example.com"} {
			resp := alice.post("/friends", url.Values{"email": {email}}, false)
			Expect(resp.StatusCode).To(Equal(http.StatusSeeOther), resp.body)
		}

		body := alice.get(group + "/settings").body
		Expect(body).To(ContainSubstring(`data-combo="friend-emails"`))
		Expect(body).To(ContainSubstring(`data-value="bob@example.com"`))
		Expect(body).To(ContainSubstring(`data-value="carol@example.com"`))

		addBob()
		body = alice.get(group + "/settings").body
		Expect(body).NotTo(ContainSubstring(`data-value="bob@example.com"`))
		Expect(body).To(ContainSubstring(`data-value="carol@example.com"`))
	})

	It("rejects adding an existing member", func() {
		addBob()
		resp := alice.post(group+"/members", url.Values{"email": {"bob@example.com"}}, false)
		Expect(resp.body).To(ContainSubstring("Already a member"))
	})

	It("lets people join with the invite link", func(ctx SpecContext) {
		mustCreateUser(ctx, "carol@example.com")
		carol := env.newClient()
		carol.login("carol@example.com")

		settings := alice.get(group + "/settings").body
		code := regexp.MustCompile(`/join/([A-Za-z0-9_-]+)`).FindStringSubmatch(settings)
		Expect(code).To(HaveLen(2))

		Expect(carol.get(group).StatusCode).To(Equal(http.StatusNotFound))
		Expect(text(carol.get("/join/" + code[1]).body)).To(ContainSubstring("Join Trip"))

		resp := carol.post("/join/"+code[1], nil, false)
		Expect(resp.Header.Get("Location")).To(Equal(group))
		Expect(carol.get(group).StatusCode).To(Equal(http.StatusOK))

		// Visiting the link again goes straight to the group.
		Expect(carol.get("/join/" + code[1]).Header.Get("Location")).To(Equal(group))
	})

	It("returns 404 for invalid invite codes", func() {
		Expect(alice.get("/join/nope").StatusCode).To(Equal(http.StatusNotFound))
	})

	It("hides groups from non-members", func(ctx SpecContext) {
		mustCreateUser(ctx, "mallory@example.com")
		mallory := env.newClient()
		mallory.login("mallory@example.com")

		for _, path := range []string{group, group + "/settings", group + "/expenses/new", group + "/settle"} {
			Expect(mallory.get(path).StatusCode).To(Equal(http.StatusNotFound), path)
		}

		resp := mallory.post(group+"/expenses", url.Values{"description": {"x"}, "amount": {"1"}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusNotFound))

		resp = mallory.post(group+"/delete", nil, true)
		Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
	})

	It("updates settings", func() {
		resp := alice.post(group+"/settings", url.Values{"name": {"Lisbon"}, "currency": {"USD"}, "simplify_debts": {"true"}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))

		body := alice.get(group + "/settings").body
		Expect(body).To(ContainSubstring(`value="Lisbon"`))
		Expect(body).To(MatchRegexp(`name="simplify_debts" value="true" checked`))
	})

	It("blocks leaving until settled up", func() {
		addBob()
		alice.addExpense(group, url.Values{"amount": {"10"}, "preset": {"you-paid-equal"}})

		resp := bob.post(group+"/leave", nil, true)
		Expect(resp.StatusCode).To(Equal(http.StatusPreconditionFailed))
		Expect(resp.body).To(ContainSubstring("Settle up your balances"))

		resp = bob.post(group+"/settle", url.Values{
			"from": {bobU.ID.String()}, "to": {aliceU.ID.String()},
			"amount": {"5"}, "currency": {"EUR"}, "date": {"2026-11-03"},
		}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))

		resp = bob.post(group+"/leave", nil, true)
		Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
		Expect(resp.Header.Get("HX-Redirect")).To(Equal("/"))
		Expect(bob.get(group).StatusCode).To(Equal(http.StatusNotFound))

		// Former members still show up by name in old expenses.
		Expect(text(alice.get(group).body)).To(ContainSubstring("bob paid you €5.00"))
	})

	It("lets any member delete a settled group, even after the creator left", func() {
		addBob()
		resp := alice.post(group+"/leave", nil, true)
		Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

		// Bob is the last member and not the creator.
		body := bob.get(group + "/settings").body
		Expect(body).To(MatchRegexp(`<button type="button" class="btn btn-danger" hx-post="[^"]+/delete"[^>]*>Delete group`))

		resp = bob.post(group+"/delete", nil, true)
		Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
		Expect(bob.get(group).StatusCode).To(Equal(http.StatusNotFound))
	})

	It("records payments with former members who still have debts", func(ctx SpecContext) {
		addBob()
		carol := mustCreateUser(ctx, "carol@example.com")
		Expect(alice.post(group+"/members", url.Values{"email": {"carol@example.com"}}, false).StatusCode).To(Equal(http.StatusSeeOther))
		resp := alice.post(group+"/settings", url.Values{"name": {"Trip"}, "currency": {"EUR"}, "simplify_debts": {"true"}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))

		// Bob owes Alice 10, Carol owes Bob 10; Carol pays Alice and Bob leaves.
		alice.addExpense(group, url.Values{"amount": {"10"}, "mode": {"advanced"}, "split_type": {"equal"},
			"paid_by": {aliceU.ID.String()}, "included": {bobU.ID.String()}})
		alice.addExpense(group, url.Values{"amount": {"10"}, "mode": {"advanced"}, "split_type": {"equal"},
			"paid_by": {bobU.ID.String()}, "included": {carol.ID.String()}})
		Expect(alice.post(group+"/settle", url.Values{
			"from": {carol.ID.String()}, "to": {aliceU.ID.String()},
			"amount": {"10"}, "currency": {"EUR"}, "date": {"2026-11-03"},
		}, false).StatusCode).To(Equal(http.StatusSeeOther))
		Expect(bob.post(group+"/leave", nil, true).StatusCode).To(Equal(http.StatusNoContent))

		// Without simplification the debts with Bob show up again.
		resp = alice.post(group+"/settings", url.Values{"name": {"Trip"}, "currency": {"EUR"}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
		Expect(text(alice.get(group).body)).To(ContainSubstring("bob owes you €10.00"))

		form := alice.get(group + "/settle?from=" + bobU.ID.String() + "&to=" + aliceU.ID.String() + "&amount=10")
		Expect(form.body).To(MatchRegexp(`<option value="` + bobU.ID.String() + `" selected>bob \(left the group\)</option>`))

		resp = alice.post(group+"/settle", url.Values{
			"from": {bobU.ID.String()}, "to": {aliceU.ID.String()},
			"amount": {"10"}, "currency": {"EUR"}, "date": {"2026-11-03"},
		}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
		Expect(text(alice.get(group).body)).NotTo(ContainSubstring("bob owes you"))

		// Carol still owes Bob, but people outside the group can not be chosen.
		Expect(text(alice.get(group).body)).To(ContainSubstring("carol owes bob €10.00"))
		mallory := mustCreateUser(ctx, "mallory@example.com")
		Expect(alice.post(group+"/settle", url.Values{
			"from": {mallory.ID.String()}, "to": {aliceU.ID.String()},
			"amount": {"1"}, "currency": {"EUR"}, "date": {"2026-11-03"},
		}, false).StatusCode).To(Equal(http.StatusOK))
	})

	It("does not delete a group with open balances", func() {
		addBob()
		alice.addExpense(group, url.Values{"amount": {"10"}, "preset": {"you-paid-equal"}})
		Expect(alice.post(group+"/delete", nil, true).StatusCode).To(Equal(http.StatusPreconditionFailed))
	})

	It("allows leaving and deleting once simplified debts are settled", func(ctx SpecContext) {
		addBob()
		carol := mustCreateUser(ctx, "carol@example.com")
		Expect(alice.post(group+"/members", url.Values{"email": {"carol@example.com"}}, false).StatusCode).To(Equal(http.StatusSeeOther))
		resp := alice.post(group+"/settings", url.Values{"name": {"Trip"}, "currency": {"EUR"}, "simplify_debts": {"true"}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))

		// Bob owes Alice 10 and Carol owes Bob 10: simplified, Carol pays Alice.
		alice.addExpense(group, url.Values{"amount": {"10"}, "mode": {"advanced"}, "split_type": {"equal"},
			"paid_by": {aliceU.ID.String()}, "included": {bobU.ID.String()}})
		alice.addExpense(group, url.Values{"amount": {"10"}, "mode": {"advanced"}, "split_type": {"equal"},
			"paid_by": {bobU.ID.String()}, "included": {carol.ID.String()}})
		Expect(text(alice.get(group).body)).To(ContainSubstring("carol owes you €10.00"))

		resp = alice.post(group+"/settle", url.Values{
			"from": {carol.ID.String()}, "to": {aliceU.ID.String()},
			"amount": {"10"}, "currency": {"EUR"}, "date": {"2026-11-03"},
		}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
		Expect(text(alice.get(group).body)).To(ContainSubstring("Everyone is settled up."))

		body := alice.get(group + "/settings").body
		Expect(body).NotTo(MatchRegexp(`disabled[^>]*>(Leave|Delete) group`))
		Expect(text(body)).NotTo(ContainSubstring("must be settled up"))

		resp = bob.post(group+"/leave", nil, true)
		Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

		resp = alice.post(group+"/delete", nil, true)
		Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
		Expect(alice.get(group).StatusCode).To(Equal(http.StatusNotFound))
	})

	It("explains why leaving or deleting is not possible", func() {
		// Alice is the only member.
		body := alice.get(group + "/settings").body
		Expect(body).To(MatchRegexp(`<button type="button" class="btn btn-danger" disabled[^>]*>Leave group`))
		Expect(body).To(MatchRegexp(`<button type="button" class="btn btn-danger" hx-post="[^"]+/delete"[^>]*>Delete group`))
		Expect(text(body)).To(ContainSubstring("You're the only member, so you can't leave. Delete the group instead."))

		addBob()
		alice.addExpense(group, url.Values{"amount": {"10"}, "preset": {"you-paid-equal"}})

		body = alice.get(group + "/settings").body
		Expect(body).To(MatchRegexp(`<button type="button" class="btn btn-danger" disabled[^>]*>Delete group`))
		Expect(text(body)).To(ContainSubstring("Settle up your balances in this group before leaving it."))
		Expect(text(body)).To(ContainSubstring("Everyone must be settled up before the group can be deleted."))

		// Bob is not the creator but can delete too once everyone is settled.
		t := text(bob.get(group + "/settings").body)
		Expect(t).To(ContainSubstring("Delete group"))
		Expect(t).To(ContainSubstring("Everyone must be settled up"))
	})
})
