package handler_test

import (
	"net/http"
	"net/url"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("authentication", func() {
	var env *testEnv

	BeforeEach(func() {
		env = newTestEnv(false)
	})

	It("redirects anonymous users to the login page", func() {
		resp := env.newClient().get("/groups/new")
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
		Expect(resp.Header.Get("Location")).To(Equal("/login?next=%2Fgroups%2Fnew"))
	})

	It("lets the first user sign up with a magic link", func() {
		c := env.newClient()
		user := c.login("Alice@Example.com")
		Expect(user.Email).To(Equal("alice@example.com"))

		resp := c.get("/")
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(text(resp.body)).To(ContainSubstring("Hi, alice"))
	})

	It("does not sign up unknown emails when signup is disabled", func(ctx SpecContext) {
		mustCreateUser(ctx, "alice@example.com")

		resp := env.newClient().post("/login", url.Values{"email": {"mallory@example.com"}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.body).To(ContainSubstring("Check your email"))
		Expect(env.mailer.messages()).To(BeEmpty())
	})

	It("signs up unknown emails when signup is enabled", func(ctx SpecContext) {
		env = newTestEnv(true)
		mustCreateUser(ctx, "alice@example.com")

		env.newClient().login("bob@example.com")
	})

	It("re-renders the form on invalid email", func() {
		resp := env.newClient().post("/login", url.Values{"email": {"nope"}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.body).To(ContainSubstring("Invalid email address"))
	})

	It("rate limits sign-in emails per address", func(ctx SpecContext) {
		mustCreateUser(ctx, "alice@example.com")
		c := env.newClient()

		c.post("/login", url.Values{"email": {"alice@example.com"}}, false)
		c.post("/login", url.Values{"email": {"alice@example.com"}}, false)
		Expect(env.mailer.messages()).To(HaveLen(1))

		env.clock.Advance(time.Minute)
		c.post("/login", url.Values{"email": {"alice@example.com"}}, false)
		Expect(env.mailer.messages()).To(HaveLen(2))
	})

	It("requires confirming the link with POST and allows it only once", func(ctx SpecContext) {
		mustCreateUser(ctx, "alice@example.com")
		c := env.newClient()
		c.post("/login", url.Values{"email": {"alice@example.com"}}, false)
		link := env.mailer.lastLinkTo("alice@example.com")

		// GET does not consume the token (email link scanners).
		Expect(c.get(link).body).To(ContainSubstring("Continue as"))
		Expect(c.get(link).body).To(ContainSubstring("Continue as"))

		resp := c.post(link, nil, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
		Expect(resp.Header.Get("Location")).To(Equal("/"))

		other := env.newClient()
		resp = other.post(link, nil, false)
		Expect(resp.body).To(ContainSubstring("Link expired"))
		Expect(other.get("/").StatusCode).To(Equal(http.StatusSeeOther))
	})

	It("rejects expired links", func(ctx SpecContext) {
		mustCreateUser(ctx, "alice@example.com")
		c := env.newClient()
		c.post("/login", url.Values{"email": {"alice@example.com"}}, false)
		link := env.mailer.lastLinkTo("alice@example.com")

		env.clock.Advance(16 * time.Minute)
		Expect(c.post(link, nil, false).body).To(ContainSubstring("Link expired"))
	})

	DescribeTable("redirects to a safe next path after login",
		func(next, expected string) {
			c := env.newClient()
			c.post("/login", url.Values{"email": {"alice@example.com"}, "next": {next}}, false)
			resp := c.post(env.mailer.lastLinkTo("alice@example.com"), nil, false)
			Expect(resp.Header.Get("Location")).To(Equal(expected))
		},
		Entry("local path", "/activity", "/activity"),
		Entry("protocol relative", "//evil.example", "/"),
		Entry("backslash", "/\\evil.example", "/"),
		Entry("absolute", "https://evil.example", "/"),
	)

	It("renews the session on login and destroys it on logout", func() {
		c := env.newClient()
		c.login("alice@example.com")

		resp := c.post("/logout", nil, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
		Expect(c.get("/").StatusCode).To(Equal(http.StatusSeeOther))
	})

	It("rejects cross-origin form posts", func() {
		c := env.newClient()
		c.login("alice@example.com")

		req, err := http.NewRequest(http.MethodPost, env.ts.URL+"/groups", nil)
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		req.Header.Set("Origin", "https://evil.example")

		resp := c.do(req)
		Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
	})
})
