package handler_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/model"
	"github.com/mgnsk/splitgo/push"
	"github.com/mgnsk/webkit/snowflake"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// sentPush is a notification sent to a user.
type sentPush struct {
	UserID snowflake.ID
	push.Message
}

// fakeNotifier records notifications.
type fakeNotifier struct {
	mu   sync.Mutex
	sent []sentPush
}

func (n *fakeNotifier) Notify(_ context.Context, userIDs []snowflake.ID, msg push.Message) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, id := range userIDs {
		n.sent = append(n.sent, sentPush{UserID: id, Message: msg})
	}
}

func (*fakeNotifier) PublicKey() string {
	return "test-vapid-key"
}

func (n *fakeNotifier) to(id snowflake.ID) []push.Message {
	n.mu.Lock()
	defer n.mu.Unlock()
	var msgs []push.Message
	for _, s := range n.sent {
		if s.UserID == id {
			msgs = append(msgs, s.Message)
		}
	}
	return msgs
}

func (c *client) postJSON(path, body string) response {
	GinkgoHelper()
	req, err := http.NewRequest(http.MethodPost, c.env.ts.URL+path, strings.NewReader(body))
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Content-Type", "application/json")
	return c.do(req)
}

const appleEndpoint = "https://web.push.apple.com/QGx3abc"

func subscriptionBody(endpoint string) string {
	return `{"endpoint":"` + endpoint + `","keys":{"p256dh":"BNcR","auth":"tBHI"}}`
}

func listSubs(ctx context.Context, ids ...snowflake.ID) []*model.PushSubscription {
	GinkgoHelper()
	subs, err := model.ListPushSubscriptions(ctx, db, ids)
	Expect(err).NotTo(HaveOccurred())
	return subs
}

var _ = Describe("push notifications", func() {
	var (
		env    *testEnv
		alice  *client
		aliceU *domain.User
		bob    *client
		bobU   *domain.User
		group  string
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
		bob = env.newClient()
		bobU = bob.login("bob@example.com")
	}

	It("shows the notification settings with the VAPID key", func() {
		body := alice.get("/account").body
		Expect(body).To(ContainSubstring(`data-vapid-key="test-vapid-key"`))
		Expect(body).To(ContainSubstring("Add to Home Screen"))
	})

	It("offers notifications on pages other than the account page", func() {
		Expect(alice.get("/").body).To(ContainSubstring(`id="push-prompt"`))
		Expect(text(alice.get(group).body)).To(ContainSubstring("You can change this later in Account settings."))
		Expect(alice.get("/account").body).NotTo(ContainSubstring(`id="push-prompt"`))
		Expect(env.newClient().get("/login").body).NotTo(ContainSubstring(`id="push-prompt"`))
	})

	It("serves the VAPID key to signed in users", func() {
		resp := alice.get("/push/key")
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.body).To(Equal("test-vapid-key"))
		Expect(env.newClient().get("/push/key").body).NotTo(Equal("test-vapid-key"))
	})

	It("stores, moves and deletes subscriptions", func(ctx SpecContext) {
		addBob()

		Expect(alice.postJSON("/push/subscribe", subscriptionBody(appleEndpoint)).StatusCode).To(Equal(http.StatusNoContent))
		subs := listSubs(ctx, aliceU.ID)
		Expect(subs).To(HaveLen(1))
		Expect(subs[0].Endpoint).To(Equal(appleEndpoint))
		Expect(subs[0].P256dh).To(Equal("BNcR"))

		// Bob can not remove Alice's subscription.
		Expect(bob.postJSON("/push/unsubscribe", subscriptionBody(appleEndpoint)).StatusCode).To(Equal(http.StatusNoContent))
		Expect(listSubs(ctx, aliceU.ID)).To(HaveLen(1))

		// The same device signed in as Bob now notifies Bob only.
		Expect(bob.postJSON("/push/subscribe", subscriptionBody(appleEndpoint)).StatusCode).To(Equal(http.StatusNoContent))
		Expect(listSubs(ctx, aliceU.ID)).To(BeEmpty())
		Expect(listSubs(ctx, bobU.ID)).To(HaveLen(1))

		Expect(bob.postJSON("/push/unsubscribe", subscriptionBody(appleEndpoint)).StatusCode).To(Equal(http.StatusNoContent))
		Expect(listSubs(ctx, bobU.ID)).To(BeEmpty())
	})

	It("rejects endpoints of unknown push services", func(ctx SpecContext) {
		for _, endpoint := range []string{
			"https://example.com/push",
			"http://web.push.apple.com/x",
			"https://push.apple.com.evil.test/x",
			"https://127.0.0.1/x",
		} {
			resp := alice.postJSON("/push/subscribe", subscriptionBody(endpoint))
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), endpoint)
		}
		Expect(alice.postJSON("/push/subscribe", `{"endpoint":"`+appleEndpoint+`"}`).StatusCode).To(Equal(http.StatusBadRequest))
		Expect(listSubs(ctx, aliceU.ID)).To(BeEmpty())
	})

	It("requires signing in", func() {
		resp := env.newClient().postJSON("/push/subscribe", subscriptionBody(appleEndpoint))
		Expect(resp.StatusCode).NotTo(Equal(http.StatusNoContent))
	})

	It("notifies a member added to a group", func() {
		addBob()
		Expect(env.notifier.to(bobU.ID)).To(ConsistOf(push.Message{
			Title: "Trip",
			Body:  "alice added you to Trip",
			URL:   group,
		}))
	})

	It("notifies others involved in a new expense", func() {
		addBob()
		alice.addExpense(group, url.Values{"amount": {"30"}, "preset": {"you-paid-equal"}})

		Expect(env.notifier.to(aliceU.ID)).To(BeEmpty())
		msgs := env.notifier.to(bobU.ID)
		Expect(msgs).To(HaveLen(2)) // invite and expense
		Expect(msgs[1].Title).To(Equal("Trip"))
		Expect(msgs[1].Body).To(Equal(`alice added "Dinner": €30.00, your share €15.00`))
		Expect(msgs[1].URL).To(MatchRegexp(`^` + group + `/expenses/\d+$`))
	})

	It("notifies the other side of a payment", func() {
		addBob()
		resp := bob.post(group+"/settle", url.Values{
			"from": {bobU.ID.String()}, "to": {aliceU.ID.String()},
			"amount": {"5"}, "currency": {"EUR"}, "date": {"2026-11-03"},
		}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))

		msgs := env.notifier.to(aliceU.ID)
		Expect(msgs).To(HaveLen(1))
		Expect(msgs[0].Body).To(Equal("bob paid you €5.00"))
		Expect(msgs[0].URL).To(Equal(group))
	})
})
