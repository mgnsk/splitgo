package handler_test

import (
	"net/http"
	"net/url"
	"regexp"

	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var inviteCodeRe = regexp.MustCompile(`/join/([A-Za-z0-9_-]+)`)

var _ = Describe("group invites, notifications and friend removal", func() {
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

		Expect(alice.post(group+"/members", url.Values{"email": {"bob@example.com"}}, false).StatusCode).To(Equal(http.StatusSeeOther))
		bob = env.newClient()
		bobU = bob.login("bob@example.com")
	})

	inviteCode := func() string {
		GinkgoHelper()
		m := inviteCodeRe.FindStringSubmatch(alice.get(group + "/settings").body)
		Expect(m).To(HaveLen(2))
		return m[1]
	}

	It("resets the invite link", func() {
		old := inviteCode()

		resp := alice.post(group+"/invite/reset", nil, true)
		Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
		Expect(resp.Header.Get("HX-Redirect")).To(Equal(group + "/settings"))

		code := inviteCode()
		Expect(code).NotTo(Equal(old))
		Expect(alice.get("/join/" + old).StatusCode).To(Equal(http.StatusNotFound))
		Expect(alice.get("/join/" + code).StatusCode).To(Equal(http.StatusSeeOther)) // Already a member.
	})

	It("lets new people sign up through a valid invite link when signup is disabled", func(ctx SpecContext) {
		code := inviteCode()
		carol := env.newClient()

		resp := carol.get("/join/" + code)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
		Expect(resp.Header.Get("Location")).To(Equal("/login?next=" + url.QueryEscape("/join/"+code)))

		resp = carol.post("/login", url.Values{"email": {"carol@example.com"}, "next": {"/join/" + code}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		resp = carol.post(env.mailer.lastLinkTo("carol@example.com"), nil, false)
		Expect(resp.Header.Get("Location")).To(Equal("/join/" + code))
		Expect(carol.post("/join/"+code, nil, false).Header.Get("Location")).To(Equal(group))

		// Unknown or reset invite codes do not allow signing up.
		sent := len(env.mailer.messages())
		for _, next := range []string{"/join/nope", "/join/", "/join/" + code + "/x", "/groups"} {
			env.newClient().post("/login", url.Values{"email": {"dave@example.com"}, "next": {next}}, false)
		}
		Expect(env.mailer.messages()).To(HaveLen(sent))
		_, err := model.GetUserByEmail(ctx, db, "dave@example.com")
		Expect(err).To(HaveOccurred())
	})

	It("notifies about edited and deleted expenses", func() {
		alice.addExpense(group, url.Values{"description": {"Dinner"}, "amount": {"30"}, "preset": {"you-paid-equal"}})
		page := alice.get(group).body
		path := regexp.MustCompile(`href="(` + group + `/expenses/\d+)"`).FindStringSubmatch(page)[1]

		resp := alice.post(path, url.Values{
			"description": {"Dinner"}, "amount": {"40"}, "currency": {"EUR"}, "date": {"2026-11-03"},
			"preset": {"you-paid-equal"},
		}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))

		msgs := env.notifier.to(bobU.ID)
		Expect(msgs[len(msgs)-1].Body).To(Equal(`alice edited "Dinner": €40.00, your share €20.00`))
		Expect(msgs[len(msgs)-1].URL).To(Equal(path))

		Expect(alice.post(path+"/delete", nil, true).StatusCode).To(Equal(http.StatusNoContent))
		msgs = env.notifier.to(bobU.ID)
		Expect(msgs[len(msgs)-1].Body).To(Equal(`alice deleted "Dinner" (€40.00)`))
		Expect(msgs[len(msgs)-1].URL).To(Equal(group))
		Expect(env.notifier.to(aliceU.ID)).To(BeEmpty())
	})

	It("tells people removed from an edited expense", func() {
		alice.addExpense(group, url.Values{"description": {"Taxi"}, "amount": {"10"}, "preset": {"you-paid-equal"}})
		path := regexp.MustCompile(`href="(` + group + `/expenses/\d+)"`).FindStringSubmatch(alice.get(group).body)[1]

		resp := alice.post(path, url.Values{
			"description": {"Taxi"}, "amount": {"10"}, "currency": {"EUR"}, "date": {"2026-11-03"},
			"mode": {"advanced"}, "split_type": {"equal"},
			"paid_by": {aliceU.ID.String()}, "included": {aliceU.ID.String()},
		}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))

		msgs := env.notifier.to(bobU.ID)
		Expect(msgs[len(msgs)-1].Body).To(Equal(`alice edited "Taxi": €10.00, you're no longer part of it`))
	})

	It("notifies about deleted payments", func() {
		Expect(bob.post(group+"/settle", url.Values{
			"from": {bobU.ID.String()}, "to": {aliceU.ID.String()},
			"amount": {"5"}, "currency": {"EUR"}, "date": {"2026-11-03"},
		}, false).StatusCode).To(Equal(http.StatusSeeOther))
		path := regexp.MustCompile(`href="(` + group + `/expenses/\d+)"`).FindStringSubmatch(bob.get(group).body)[1]

		Expect(bob.post(path+"/delete", nil, true).StatusCode).To(Equal(http.StatusNoContent))
		msgs := env.notifier.to(aliceU.ID)
		Expect(msgs[len(msgs)-1].Body).To(Equal("bob deleted a payment of €5.00"))
	})

	It("asks before removing a friend", func() {
		resp := alice.post("/friends", url.Values{"email": {"bob@example.com"}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))

		body := alice.get(resp.Header.Get("Location")).body
		Expect(body).To(MatchRegexp(`hx-post="/friends/` + bobU.ID.String() + `/remove" hx-confirm="Remove bob and delete your non-group expenses with them for both of you\?"`))
	})
})
