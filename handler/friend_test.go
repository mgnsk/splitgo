package handler_test

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/model"
	"github.com/mgnsk/splitgo/pkg/snowflake"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var addExpenseRe = regexp.MustCompile(`href="(/groups/\d+)/expenses/new"`)

var _ = Describe("friends", func() {
	var (
		env    *testEnv
		alice  *client
		aliceU *domain.User
		bob    *client
		bobU   *domain.User
	)

	// addFriend adds a friend by email and returns the friend page path.
	addFriend := func(c *client, email string) string {
		GinkgoHelper()
		resp := c.post("/friends", url.Values{"email": {email}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther), resp.body)
		return resp.Header.Get("Location")
	}

	// ledger returns the path of the hidden group with a friend.
	ledger := func(c *client, friendPath string) string {
		GinkgoHelper()
		m := addExpenseRe.FindStringSubmatch(c.get(friendPath).body)
		Expect(m).To(HaveLen(2))
		return m[1]
	}

	BeforeEach(func() {
		env = newTestEnv(false)
		alice = env.newClient()
		aliceU = alice.login("alice@example.com")
	})

	addBob := func() string {
		GinkgoHelper()
		path := addFriend(alice, "Bob@Example.com")
		bob = env.newClient()
		resp := bob.post(env.mailer.lastLinkTo("bob@example.com"), nil, false)
		Expect(resp.Header.Get("Location")).To(Equal("/friends/" + aliceU.ID.String()))
		user, err := model.GetUserByEmail(GinkgoT().Context(), db, "bob@example.com")
		Expect(err).NotTo(HaveOccurred())
		bobU = user
		Expect(path).To(Equal("/friends/" + bobU.ID.String()))
		return path
	}

	It("adds a friend by email and invites them", func() {
		addBob()

		msgs := env.mailer.messages()
		Expect(msgs[len(msgs)-1].Subject).To(Equal("alice added you as a friend on splitgo"))
		Expect(env.notifier.to(bobU.ID)).To(HaveLen(1))

		Expect(text(alice.get("/").body)).To(ContainSubstring("Friends b bob settled up + Add friend"))
		Expect(text(bob.get("/").body)).To(ContainSubstring("Friends a alice settled up + Add friend"))
	})

	It("does not add a friend twice or yourself", func() {
		addBob()
		sent := len(env.mailer.messages())

		Expect(addFriend(alice, "bob@example.com")).To(Equal("/friends/" + bobU.ID.String()))
		Expect(addFriend(bob, "alice@example.com")).To(Equal("/friends/" + aliceU.ID.String()))
		Expect(env.mailer.messages()).To(HaveLen(sent))

		resp := alice.post("/friends", url.Values{"email": {"ALICE@example.com"}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(text(resp.body)).To(ContainSubstring("That's your own email address"))

		resp = alice.post("/friends", url.Values{"email": {"nope"}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	It("adds expenses with a friend without a group", func() {
		friendPage := addBob()
		g := ledger(alice, friendPage)

		// The expense form offers the two-person presets.
		Expect(alice.get(g + "/expenses/new").body).To(ContainSubstring(`value="you-paid-equal"`))

		resp := alice.post(g+"/expenses", url.Values{
			"description": {"Lunch"}, "amount": {"20"}, "currency": {"EUR"}, "date": {"2026-11-03"},
			"preset": {"you-paid-equal"},
		}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
		Expect(resp.Header.Get("Location")).To(Equal(friendPage))

		t := text(alice.get(friendPage).body)
		Expect(t).To(ContainSubstring("bob owes you €10.00"))
		Expect(t).To(ContainSubstring("Settle up You and bob bob owes you €10.00 Settle up"))
		Expect(t).To(ContainSubstring("Lunch You paid €20.00"))

		Expect(text(alice.get("/").body)).To(ContainSubstring("bob you are owed €10.00"))
		Expect(text(bob.get("/").body)).To(ContainSubstring("alice you owe €10.00"))

		// The other side is notified with the friend as the title.
		msgs := env.notifier.to(bobU.ID)
		Expect(msgs[len(msgs)-1].Title).To(Equal("alice"))

		// Expense pages link back to the friend, not a group.
		Expect(alice.get(g + "/expenses/new").body).To(ContainSubstring(`<a href="` + friendPage + `">bob</a>`))
	})

	It("settles non-group and group debts separately", func(ctx SpecContext) {
		friendPage := addBob()
		g := ledger(alice, friendPage)
		alice.addExpense(g, url.Values{"amount": {"20"}, "preset": {"you-paid-equal"}})

		trip := alice.createGroup("Trip")
		var tripID snowflake.ID
		Expect(tripID.UnmarshalText([]byte(strings.TrimPrefix(trip, "/groups/")))).To(Succeed())
		Expect(model.AddGroupMember(ctx, db, tripID, bobU.ID)).To(Succeed())
		bob.addExpense(trip, url.Values{"amount": {"6"}, "preset": {"you-paid-equal"}})

		page := alice.get(friendPage).body
		t := text(page)
		Expect(t).To(ContainSubstring("bob owes you €7.00"))
		Expect(t).To(ContainSubstring("You and bob bob owes you €10.00"))
		Expect(t).To(ContainSubstring("Trip You owe bob €3.00"))
		Expect(page).To(ContainSubstring(g + "/settle?amount=10.00&amp;currency=EUR&amp;from=" + bobU.ID.String() + "&amp;to=" + aliceU.ID.String()))

		resp := bob.post(g+"/settle", url.Values{
			"from": {bobU.ID.String()}, "to": {aliceU.ID.String()},
			"amount": {"10"}, "currency": {"EUR"}, "date": {"2026-11-04"},
		}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
		Expect(resp.Header.Get("Location")).To(Equal("/friends/" + aliceU.ID.String()))

		t = text(alice.get(friendPage).body)
		Expect(t).NotTo(ContainSubstring("You and bob"))
		Expect(t).To(ContainSubstring("Trip You owe bob €3.00"))
	})

	It("hides the friend ledger as a group", func() {
		friendPage := addBob()
		g := ledger(alice, friendPage)
		alice.createGroup("Trip")

		home := text(alice.get("/").body)
		Expect(home).To(ContainSubstring("Groups T Trip settled up + New group"))

		chooser := alice.get("/expenses/new").body
		Expect(chooser).To(ContainSubstring(`href="` + g + `/expenses/new"`))
		Expect(text(chooser)).To(ContainSubstring("Friends b bob + Add friend Groups T Trip + New group"))

		resp := alice.get(g)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
		Expect(resp.Header.Get("Location")).To(Equal(friendPage))

		Expect(alice.get(g + "/settings").StatusCode).To(Equal(http.StatusNotFound))
		for _, path := range []string{"/settings", "/members", "/leave", "/delete"} {
			Expect(alice.post(g+path, url.Values{"name": {"x"}, "email": {"c@example.com"}}, false).StatusCode).To(Equal(http.StatusNotFound), path)
		}
	})

	It("removes a settled friend only", func() {
		friendPage := addBob()
		g := ledger(alice, friendPage)
		alice.addExpense(g, url.Values{"amount": {"20"}, "preset": {"you-paid-equal"}})

		Expect(alice.get(friendPage).body).NotTo(ContainSubstring("Remove friend"))
		Expect(alice.post(friendPage+"/remove", nil, true).StatusCode).To(Equal(http.StatusPreconditionFailed))

		alice.addExpense(g, url.Values{"amount": {"20"}, "preset": {"they-paid-equal"}})
		Expect(alice.get(friendPage).body).To(ContainSubstring("Remove friend"))

		resp := alice.post(friendPage+"/remove", nil, true)
		Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
		Expect(resp.Header.Get("HX-Redirect")).To(Equal("/"))
		Expect(text(alice.get("/").body)).To(ContainSubstring("Friends + Add friend"))
		Expect(alice.get(g + "/expenses/new").StatusCode).To(Equal(http.StatusNotFound))
	})

	It("shows group members who are not friends but hides strangers", func(ctx SpecContext) {
		carol := mustCreateUser(ctx, "carol@example.com")
		trip := alice.createGroup("Trip")
		var tripID snowflake.ID
		Expect(tripID.UnmarshalText([]byte(strings.TrimPrefix(trip, "/groups/")))).To(Succeed())
		Expect(model.AddGroupMember(ctx, db, tripID, carol.ID)).To(Succeed())
		alice.addExpense(trip, url.Values{"amount": {"8"}, "preset": {"you-paid-equal"}})

		home := text(alice.get("/").body)
		Expect(home).To(ContainSubstring("Other balances c carol owes you €4.00"))

		page := alice.get("/friends/" + carol.ID.String()).body
		Expect(text(page)).To(ContainSubstring("carol owes you €4.00"))
		Expect(page).To(ContainSubstring(`<input type="hidden" name="email" value="carol@example.com">`))

		stranger := mustCreateUser(ctx, "mallory@example.com")
		Expect(alice.get("/friends/abc").StatusCode).To(Equal(http.StatusBadRequest))
		for _, id := range []string{stranger.ID.String(), aliceU.ID.String(), "0"} {
			Expect(alice.get("/friends/"+id).StatusCode).To(Equal(http.StatusNotFound), id)
		}
	})

	It("labels non-group expenses with the friend in activity", func() {
		friendPage := addBob()
		g := ledger(alice, friendPage)
		alice.addExpense(g, url.Values{"description": {"Lunch"}, "amount": {"20"}, "preset": {"you-paid-equal"}})

		Expect(text(alice.get("/activity").body)).To(ContainSubstring("Lunch bob · You paid €20.00"))
		Expect(text(bob.get("/activity").body)).To(ContainSubstring("Lunch alice · alice paid €20.00"))
	})
})

var _ = Describe("invite limit", func() {
	It("limits invite emails per day across friends and groups", func(ctx SpecContext) {
		env := newTestEnv(false)
		alice := env.newClient()
		alice.login("alice@example.com")
		group := alice.createGroup("Trip")

		for i := range model.MaxInvites {
			email := fmt.Sprintf("friend%d@example.com", i)
			if i%2 == 0 {
				Expect(alice.post("/friends", url.Values{"email": {email}}, false).StatusCode).To(Equal(http.StatusSeeOther))
			} else {
				Expect(alice.post(group+"/members", url.Values{"email": {email}}, false).StatusCode).To(Equal(http.StatusSeeOther))
			}
		}
		sent := len(env.mailer.messages())

		resp := alice.post("/friends", url.Values{"email": {"extra@example.com"}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(text(resp.body)).To(ContainSubstring("You can send up to 20 invites a day."))

		resp = alice.post(group+"/members", url.Values{"email": {"extra@example.com"}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(text(resp.body)).To(ContainSubstring("You can send up to 20 invites a day."))

		Expect(env.mailer.messages()).To(HaveLen(sent))
		_, err := model.GetUserByEmail(ctx, db, "extra@example.com")
		Expect(err).To(HaveOccurred(), "no account is created when refused")

		// Existing friends still open without sending anything.
		Expect(alice.post("/friends", url.Values{"email": {"friend0@example.com"}}, false).StatusCode).To(Equal(http.StatusSeeOther))

		env.clock.Advance(model.InviteWindow)
		Expect(alice.post("/friends", url.Values{"email": {"extra@example.com"}}, false).StatusCode).To(Equal(http.StatusSeeOther))
	})
})
