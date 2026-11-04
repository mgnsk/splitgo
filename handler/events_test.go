package handler_test

import (
	"bufio"
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// stream opens the event stream and returns a channel of event lines.
func (c *client) stream(ctx context.Context) <-chan string {
	GinkgoHelper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.env.ts.URL+"/events", nil)
	Expect(err).NotTo(HaveOccurred())
	resp, err := c.http.Do(req)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(resp.Header.Get("Content-Type")).To(Equal("text/event-stream"))

	lines := make(chan string, 16)
	go func() {
		defer GinkgoRecover()
		defer close(lines)
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), "event:") || strings.HasPrefix(sc.Text(), "data:") {
				lines <- sc.Text()
			}
		}
	}()
	return lines
}

var _ = Describe("live updates", func() {
	It("tells other members' open pages about changes", func(ctx SpecContext) {
		env := newTestEnv(false)
		alice := env.newClient()
		alice.login("alice@example.com")
		group := alice.createGroup("Trip")
		groupID := strings.TrimPrefix(group, "/groups/")

		Expect(alice.post(group+"/members", url.Values{"email": {"bob@example.com"}}, false).StatusCode).To(Equal(http.StatusSeeOther))
		bob := env.newClient()
		bob.login("bob@example.com")

		mustCreateUser(ctx, "carol@example.com")
		carol := env.newClient()
		carol.login("carol@example.com")

		sctx, cancel := context.WithCancel(ctx)
		defer cancel()
		aliceEvents := alice.stream(sctx)
		carolEvents := carol.stream(sctx)

		bob.addExpense(group, url.Values{"amount": {"30"}, "preset": {"you-paid-equal"}})

		Eventually(aliceEvents).WithTimeout(5 * time.Second).Should(Receive(Equal("event: refresh")))
		Eventually(aliceEvents).Should(Receive(Equal("data: " + groupID)))
		Consistently(carolEvents, 200*time.Millisecond).ShouldNot(Receive())
	})

	It("requires signing in", func() {
		env := newTestEnv(false)
		Expect(env.newClient().get("/events").StatusCode).NotTo(Equal(http.StatusOK))
	})
})
