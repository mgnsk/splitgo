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

		for _, path := range []string{group, group + "/balances", group + "/settings", group + "/expenses/new", group + "/settle"} {
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

	It("lets only the creator delete a settled group", func() {
		addBob()
		Expect(bob.post(group+"/delete", nil, true).StatusCode).To(Equal(http.StatusForbidden))

		resp := alice.post(group+"/delete", nil, true)
		Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
		Expect(alice.get(group).StatusCode).To(Equal(http.StatusNotFound))
	})

	It("does not delete a group with open balances", func() {
		addBob()
		alice.addExpense(group, url.Values{"amount": {"10"}, "preset": {"you-paid-equal"}})
		Expect(alice.post(group+"/delete", nil, true).StatusCode).To(Equal(http.StatusPreconditionFailed))
	})
})
