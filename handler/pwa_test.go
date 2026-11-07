package handler_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("sign-in codes", func() {
	var env *testEnv

	BeforeEach(func(ctx SpecContext) {
		env = newTestEnv(false)
		mustCreateUser(ctx, "alice@example.com")
	})

	request := func(c *client) string {
		GinkgoHelper()
		resp := c.post("/login", url.Values{"email": {"alice@example.com"}, "next": {"/activity"}}, false)
		Expect(resp.body).To(ContainSubstring(`name="code"`))
		Expect(resp.body).To(ContainSubstring(`autocomplete="one-time-code"`))
		return env.mailer.lastCodeTo("alice@example.com")
	}

	enter := func(c *client, code string) response {
		GinkgoHelper()
		return c.post("/login/code", url.Values{"email": {"alice@example.com"}, "code": {code}, "next": {"/activity"}}, false)
	}

	It("emails a code with the link", func() {
		request(env.newClient())
		msg := env.mailer.messages()[0]
		Expect(msg.Subject).To(MatchRegexp(`^Your splitgo sign-in code: \d{6}$`))
		Expect(msg.Body).To(MatchRegexp(`/login/[A-Za-z0-9_-]+`))
	})

	It("signs in an app that did not open the link", func() {
		app := env.newClient()
		code := request(app)

		resp := enter(app, code[:3]+" "+code[3:])
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
		Expect(resp.Header.Get("Location")).To(Equal("/activity"))
		Expect(app.get("/").StatusCode).To(Equal(http.StatusOK))

		// The code and its link are single use.
		Expect(enter(env.newClient(), code).body).To(ContainSubstring("This code has expired"))
	})

	It("rejects wrong and malformed codes", func() {
		c := env.newClient()
		code := request(c)
		wrong := "000000"
		if code == wrong {
			wrong = "111111"
		}

		Expect(enter(c, wrong).body).To(ContainSubstring("Wrong code"))
		Expect(enter(c, "12ab").body).To(ContainSubstring("Enter the 6-digit code"))
		Expect(c.get("/").StatusCode).To(Equal(http.StatusSeeOther))

		// The right code still works after a wrong one.
		Expect(enter(c, code).StatusCode).To(Equal(http.StatusSeeOther))
	})

	It("expires codes", func() {
		c := env.newClient()
		code := request(c)
		env.clock.Advance(16 * time.Minute)
		Expect(enter(c, code).body).To(ContainSubstring("This code has expired"))
	})

	It("refuses codes after too many failures but keeps the link working", func() {
		c := env.newClient()
		for range 4 {
			code := request(c)
			for range 5 {
				wrong := "000000"
				if code == wrong {
					wrong = "111111"
				}
				enter(c, wrong)
			}
			env.clock.Advance(2 * time.Minute)
		}

		code := request(c)
		Expect(enter(c, code).body).To(ContainSubstring("Too many wrong codes"))

		resp := c.post(env.mailer.lastLinkTo("alice@example.com"), nil, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
	})

	It("does not reveal unknown emails", func() {
		c := env.newClient()
		resp := c.post("/login", url.Values{"email": {"mallory@example.com"}}, false)
		Expect(resp.body).To(ContainSubstring(`name="code"`))

		resp = c.post("/login/code", url.Values{"email": {"mallory@example.com"}, "code": {"123456"}}, false)
		Expect(resp.body).To(ContainSubstring("This code has expired"))
	})
})

var _ = Describe("PWA", func() {
	var c *client

	BeforeEach(func() {
		env := newTestEnv(false)
		c = env.newClient()
	})

	It("links the manifest and icons", func() {
		body := c.get("/login").body
		Expect(body).To(MatchRegexp(`<link rel="manifest" href="/manifest.webmanifest\?crc=\d+">`))
		Expect(body).To(ContainSubstring(`rel="apple-touch-icon"`))
		Expect(body).To(ContainSubstring(`name="theme-color"`))
		Expect(body).To(MatchRegexp(`<script defer src="/assets/app.js\?crc=\d+">`))
	})

	It("serves a valid manifest", func() {
		resp := c.get("/manifest.webmanifest")
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Content-Type")).To(Equal("application/manifest+json"))

		var manifest struct {
			StartURL string `json:"start_url"`
			Display  string `json:"display"`
			Icons    []struct {
				Src     string `json:"src"`
				Purpose string `json:"purpose"`
			} `json:"icons"`
		}
		Expect(json.Unmarshal([]byte(resp.body), &manifest)).To(Succeed())
		Expect(manifest.StartURL).To(Equal("/"))
		Expect(manifest.Display).To(Equal("standalone"))
		Expect(manifest.Icons).To(ContainElement(HaveField("Purpose", "maskable")))

		for _, icon := range manifest.Icons {
			r := c.get(icon.Src)
			Expect(r.StatusCode).To(Equal(http.StatusOK), icon.Src)
			Expect(r.Header.Get("Content-Type")).To(Equal("image/png"))
		}
	})

	It("serves the service worker from the root without long caching", func() {
		resp := c.get("/sw.js")
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Cache-Control")).To(Equal("no-cache"))
		Expect(resp.Header.Get("Content-Type")).To(HavePrefix("text/javascript"))
		Expect(resp.body).To(ContainSubstring("/assets/offline.html"))

		Expect(c.get("/assets/offline.html").StatusCode).To(Equal(http.StatusOK))
	})

	It("keeps sessions for a year", func() {
		c.post("/login", url.Values{"email": {"alice@example.com"}}, false)
		resp := c.post(c.env.mailer.lastLinkTo("alice@example.com"), nil, false)

		var session *http.Cookie
		for _, ck := range resp.Cookies() {
			if ck.Name == "session_id" {
				session = ck
			}
		}
		Expect(session).NotTo(BeNil())
		Expect(session.MaxAge).To(BeNumerically("~", 365*24*60*60, 60))
	})
})
