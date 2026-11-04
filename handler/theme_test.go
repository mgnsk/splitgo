package handler_test

import (
	"context"
	"net/http"
	"net/url"

	"github.com/mgnsk/splitgo/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("theme", func() {
	var c *client

	BeforeEach(func() {
		env := newTestEnv(false)
		c = env.newClient()
		c.login("alice@example.com")
	})

	It("follows the system theme by default", func() {
		body := c.get("/").body
		Expect(body).To(ContainSubstring(`<html lang="en" data-timezone="">`))
		Expect(body).To(ContainSubstring(`value="dark"`))
		Expect(body).To(ContainSubstring(`value="light"`))
	})

	It("toggles the theme from the top menu and returns to the page", func() {
		resp := c.post("/theme", url.Values{"theme": {"dark"}, "next": {"/activity"}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
		Expect(resp.Header.Get("Location")).To(Equal("/activity"))
		Expect(c.get("/activity").body).To(ContainSubstring(`<html lang="en" data-theme="dark" data-timezone="">`))

		c.post("/theme", url.Values{"theme": {"light"}, "next": {"/"}}, false)
		Expect(c.get("/").body).To(ContainSubstring(`<html lang="en" data-theme="light" data-timezone="">`))
	})

	It("does not redirect off-site", func() {
		resp := c.post("/theme", url.Values{"theme": {"dark"}, "next": {"//evil.example"}}, false)
		Expect(resp.Header.Get("Location")).To(Equal("/"))
	})

	It("rejects unknown themes", func() {
		resp := c.post("/theme", url.Values{"theme": {"neon"}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
	})

	It("can be set back to system on the account page", func() {
		c.post("/theme", url.Values{"theme": {"dark"}}, false)
		Expect(c.get("/account").body).To(ContainSubstring(`<option value="dark" selected>`))

		resp := c.post("/account", url.Values{"name": {"Alice"}, "currency": {"EUR"}, "theme": {"system"}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
		Expect(c.get("/").body).To(ContainSubstring(`<html lang="en" data-timezone="">`))
	})

	It("requires login", func() {
		resp := newTestEnv(false).newClient().post("/theme", url.Values{"theme": {"dark"}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
	})
})

var _ = Describe("time zone", func() {
	It("uses the browser's time zone for today's date", func() {
		env := newTestEnv(false)
		c := env.newClient()
		c.login("alice@example.com")
		group := c.createGroup("Trip")

		// The test clock is 2026-11-03 12:00 UTC.
		Expect(c.get(group + "/expenses/new").body).To(ContainSubstring(`value="2026-11-03"`))

		resp := c.post("/timezone", url.Values{"timezone": {"Pacific/Kiritimati"}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
		Expect(c.get("/").body).To(ContainSubstring(`data-timezone="Pacific/Kiritimati"`))

		// UTC+14 is already on the next day.
		Expect(c.get(group + "/expenses/new").body).To(ContainSubstring(`value="2026-11-04"`))
		Expect(c.get(group + "/settle").body).To(ContainSubstring(`value="2026-11-04"`))
	})

	It("rejects unknown time zones", func() {
		env := newTestEnv(false)
		c := env.newClient()
		c.login("alice@example.com")

		for _, tz := range []string{"", "Local", "Mars/Olympus", "../../etc/passwd"} {
			Expect(c.post("/timezone", url.Values{"timezone": {tz}}, false).StatusCode).To(Equal(http.StatusBadRequest), tz)
		}
		Expect(env.newClient().post("/timezone", url.Values{"timezone": {"UTC"}}, false).StatusCode).NotTo(Equal(http.StatusNoContent))
	})
})

var _ = Describe("time zone on sign-in", func() {
	var env *testEnv

	BeforeEach(func(ctx SpecContext) {
		env = newTestEnv(false)
		mustCreateUser(ctx, "alice@example.com")
	})

	timezone := func(ctx context.Context) string {
		GinkgoHelper()
		u, err := model.GetUserByEmail(ctx, db, "alice@example.com")
		Expect(err).NotTo(HaveOccurred())
		return u.Timezone
	}

	It("is sent with the sign-in code", func(ctx SpecContext) {
		c := env.newClient()
		resp := c.post("/login", url.Values{"email": {"alice@example.com"}}, false)
		Expect(resp.body).To(ContainSubstring(`<input type="hidden" name="timezone" value="">`))

		resp = c.post("/login/code", url.Values{
			"email": {"alice@example.com"}, "code": {env.mailer.lastCodeTo("alice@example.com")},
			"timezone": {"Asia/Tokyo"},
		}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
		Expect(timezone(ctx)).To(Equal("Asia/Tokyo"))
	})

	It("is sent when confirming the sign-in link", func(ctx SpecContext) {
		c := env.newClient()
		c.post("/login", url.Values{"email": {"alice@example.com"}}, false)
		link := env.mailer.lastLinkTo("alice@example.com")
		Expect(c.get(link).body).To(ContainSubstring(`name="timezone"`))

		resp := c.post(link, url.Values{"timezone": {"America/New_York"}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
		Expect(timezone(ctx)).To(Equal("America/New_York"))
	})

	It("ignores a missing or unknown time zone", func(ctx SpecContext) {
		c := env.newClient()
		c.post("/login", url.Values{"email": {"alice@example.com"}}, false)
		resp := c.post(env.mailer.lastLinkTo("alice@example.com"), url.Values{"timezone": {"Mars/Olympus"}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
		Expect(timezone(ctx)).To(BeEmpty())
	})
})
