package handler_test

import (
	"net/http"
	"net/url"

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
		Expect(body).To(ContainSubstring(`<html lang="en">`))
		Expect(body).To(ContainSubstring(`value="dark"`))
		Expect(body).To(ContainSubstring(`value="light"`))
	})

	It("toggles the theme from the top menu and returns to the page", func() {
		resp := c.post("/theme", url.Values{"theme": {"dark"}, "next": {"/activity"}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
		Expect(resp.Header.Get("Location")).To(Equal("/activity"))
		Expect(c.get("/activity").body).To(ContainSubstring(`<html lang="en" data-theme="dark">`))

		c.post("/theme", url.Values{"theme": {"light"}, "next": {"/"}}, false)
		Expect(c.get("/").body).To(ContainSubstring(`<html lang="en" data-theme="light">`))
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
		Expect(c.get("/").body).To(ContainSubstring(`<html lang="en">`))
	})

	It("requires login", func() {
		resp := newTestEnv(false).newClient().post("/theme", url.Values{"theme": {"dark"}}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
	})
})
