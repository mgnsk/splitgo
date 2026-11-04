package handler_test

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/alexedwards/scs/bunstore"
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/handler"
	"github.com/mgnsk/splitgo/mail"
	"github.com/mgnsk/splitgo/model"
	"github.com/mgnsk/splitgo/pkg/snowflake"
	"github.com/mgnsk/splitgo/server"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// fakeMailer records sent messages.
type fakeMailer struct {
	mu   sync.Mutex
	sent []mail.Message
}

func (m *fakeMailer) Send(_ context.Context, msg mail.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

func (m *fakeMailer) messages() []mail.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]mail.Message(nil), m.sent...)
}

var linkRe = regexp.MustCompile(`https://[^\s]+/login/[A-Za-z0-9_-]+(\?next=[^\s]+)?`)

// lastLinkTo returns the path of the last sign-in link emailed to addr.
func (m *fakeMailer) lastLinkTo(addr string) string {
	GinkgoHelper()
	msgs := m.messages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].To == addr {
			link := linkRe.FindString(msgs[i].Body)
			Expect(link).NotTo(BeEmpty())
			u, err := url.Parse(link)
			Expect(err).NotTo(HaveOccurred())
			return u.RequestURI()
		}
	}
	Fail("no email sent to " + addr)
	return ""
}

// testClock is a controllable clock.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type testEnv struct {
	ts       *httptest.Server
	mailer   *fakeMailer
	notifier *fakeNotifier
	clock    *testClock
}

func newTestEnv(allowSignup bool) *testEnv {
	store, err := bunstore.NewWithCleanupInterval(db, 0)
	Expect(err).NotTo(HaveOccurred())

	env := &testEnv{
		mailer:   &fakeMailer{},
		notifier: &fakeNotifier{},
		clock:    &testClock{now: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)},
	}

	deps := &handler.Deps{
		DB:     db,
		SM:     server.NewSessionManager(store, true),
		Mailer: env.mailer,
		Push:   env.notifier,
		Config: handler.Config{
			BaseURL:     "https://splitgo.test",
			AllowSignup: allowSignup,
			Now:         env.clock.Now,
		},
	}

	env.ts = httptest.NewTLSServer(handler.New(deps))
	DeferCleanup(env.ts.Close)

	return env
}

// client is a simulated browser.
type client struct {
	env  *testEnv
	http *http.Client
}

func (env *testEnv) newClient() *client {
	jar, err := cookiejar.New(nil)
	Expect(err).NotTo(HaveOccurred())

	c := *env.ts.Client()
	c.Jar = jar
	c.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	return &client{env: env, http: &c}
}

type response struct {
	*http.Response
	body string
}

func (c *client) do(req *http.Request) response {
	GinkgoHelper()
	resp, err := c.http.Do(req)
	Expect(err).NotTo(HaveOccurred())
	body, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.Body.Close()).To(Succeed())
	return response{Response: resp, body: string(body)}
}

func (c *client) get(path string) response {
	GinkgoHelper()
	req, err := http.NewRequest(http.MethodGet, c.env.ts.URL+path, nil)
	Expect(err).NotTo(HaveOccurred())
	return c.do(req)
}

// getFollow GETs path following redirects.
func (c *client) getFollow(path string) response {
	GinkgoHelper()
	for range 5 {
		resp := c.get(path)
		if resp.StatusCode != http.StatusSeeOther && resp.StatusCode != http.StatusFound {
			return resp
		}
		path = resp.Header.Get("Location")
	}
	Fail("too many redirects")
	return response{}
}

func (c *client) post(path string, form url.Values, hx bool) response {
	GinkgoHelper()
	req, err := http.NewRequest(http.MethodPost, c.env.ts.URL+path, strings.NewReader(form.Encode()))
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if hx {
		req.Header.Set("HX-Request", "true")
	}
	return c.do(req)
}

// login signs in via a magic link and returns the user.
func (c *client) login(email string) *domain.User {
	GinkgoHelper()

	resp := c.post("/login", url.Values{"email": {email}}, false)
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(resp.body).To(ContainSubstring("Check your email"))

	link := c.env.mailer.lastLinkTo(domain.NormalizeEmail(email))
	resp = c.post(link, nil, false)
	Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))

	// Next login request from the same address must not be rate limited.
	c.env.clock.Advance(2 * time.Minute)

	user, err := model.GetUserByEmail(context.Background(), db, email)
	Expect(err).NotTo(HaveOccurred())
	return user
}

func mustCreateUser(ctx context.Context, email string) *domain.User {
	u := &domain.User{ID: snowflake.Generate(), Email: email, Currency: "EUR"}
	Expect(model.InsertUser(ctx, db, u)).To(Succeed())
	return u
}

var groupPathRe = regexp.MustCompile(`^/groups/\d+$`)

// createGroup creates a group via HTTP and returns its path.
func (c *client) createGroup(name string) string {
	GinkgoHelper()
	resp := c.post("/groups", url.Values{"name": {name}, "currency": {"EUR"}}, false)
	Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
	loc := resp.Header.Get("Location")
	Expect(loc).To(MatchRegexp(groupPathRe.String()))
	return loc
}

// addExpense posts the expense form and expects success.
func (c *client) addExpense(groupPath string, form url.Values) {
	GinkgoHelper()
	base := url.Values{
		"description": {"Dinner"},
		"currency":    {"EUR"},
		"date":        {"2026-11-03"},
	}
	for k, v := range form {
		base[k] = v
	}
	resp := c.post(groupPath+"/expenses", base, false)
	Expect(resp.StatusCode).To(Equal(http.StatusSeeOther), resp.body)
}

// text strips tags and collapses whitespace for content assertions.
func text(html string) string {
	html = regexp.MustCompile(`(?s)<head>.*</head>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(html, " ")
	html = strings.NewReplacer("&#39;", "'", "&amp;", "&", "&quot;", `"`).Replace(html)
	return strings.Join(strings.Fields(html), " ")
}

var codeRe = regexp.MustCompile(`sign-in code is:\s+(\d{6})`)

// lastCodeTo returns the last sign-in code emailed to addr.
func (m *fakeMailer) lastCodeTo(addr string) string {
	GinkgoHelper()
	msgs := m.messages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].To == addr {
			match := codeRe.FindStringSubmatch(msgs[i].Body)
			Expect(match).To(HaveLen(2))
			return match[1]
		}
	}
	Fail("no email sent to " + addr)
	return ""
}
