package push

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/mgnsk/splitgo"
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/model"
	"github.com/mgnsk/webkit/snowflake"
	"github.com/mgnsk/webkit/sqlite"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/uptrace/bun"
)

var _ = Describe("ValidEndpoint", func() {
	DescribeTable("accepts known push services only",
		func(endpoint string, ok bool) {
			Expect(ValidEndpoint(endpoint)).To(Equal(ok))
		},
		Entry(nil, "https://web.push.apple.com/abc", true),
		Entry(nil, "https://fcm.googleapis.com/fcm/send/abc", true),
		Entry(nil, "https://updates.push.services.mozilla.com/wpush/v2/abc", true),
		Entry(nil, "https://wns2-par02p.notify.windows.com/w/?token=abc", true),
		Entry(nil, "http://web.push.apple.com/abc", false),
		Entry(nil, "https://web.push.apple.com:8443/abc", false),
		Entry(nil, "https://user@web.push.apple.com/abc", false),
		Entry(nil, "https://push.apple.com.evil.test/abc", false),
		Entry(nil, "https://evilpush.apple.com.test/abc", false),
		Entry(nil, "https://localhost/abc", false),
		Entry(nil, "https://"+strings.Repeat("a", 1100)+".push.apple.com/", false),
	)
})

var _ = Describe("WebPush", func() {
	var (
		db   *bun.DB
		user *domain.User
	)

	BeforeEach(func(ctx SpecContext) {
		db = sqlite.NewDB(":memory:").WithForeignKeys().Connect()
		DeferCleanup(db.Close)
		Expect(splitgo.MigrateUp(db.DB)).To(Succeed())

		user = &domain.User{ID: snowflake.Generate(), Email: "alice@example.com", Currency: "EUR"}
		Expect(model.InsertUser(ctx, db, user)).To(Succeed())
	})

	subscribe := func(ctx SpecContext, endpoint string) {
		GinkgoHelper()
		key, err := ecdh.P256().GenerateKey(rand.Reader)
		Expect(err).NotTo(HaveOccurred())
		auth := make([]byte, 16)
		_, err = rand.Read(auth)
		Expect(err).NotTo(HaveOccurred())
		Expect(model.SavePushSubscription(ctx, db, &model.PushSubscription{
			Endpoint: endpoint,
			UserID:   user.ID,
			P256dh:   base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),
			Auth:     base64.RawURLEncoding.EncodeToString(auth),
		}, time.Now())).To(Succeed())
	}

	It("keeps its VAPID keys", func(ctx SpecContext) {
		a, err := New(ctx, db, "admin@example.com")
		Expect(err).NotTo(HaveOccurred())
		b, err := New(ctx, db, "admin@example.com")
		Expect(err).NotTo(HaveOccurred())
		Expect(a.PublicKey()).NotTo(BeEmpty())
		Expect(b.PublicKey()).To(Equal(a.PublicKey()))
	})

	It("sends encrypted notifications and drops expired subscriptions", func(ctx SpecContext) {
		var (
			mu     sync.Mutex
			reqs   []*http.Request
			bodies []int
		)
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			mu.Lock()
			reqs = append(reqs, r)
			bodies = append(bodies, len(b))
			mu.Unlock()
			if r.URL.Path == "/gone" {
				w.WriteHeader(http.StatusGone)
				return
			}
			w.WriteHeader(http.StatusCreated)
		}))
		DeferCleanup(ts.Close)

		subscribe(ctx, ts.URL+"/ok")
		subscribe(ctx, ts.URL+"/gone")

		p, err := New(ctx, db, "admin@example.com")
		Expect(err).NotTo(HaveOccurred())
		p.client = ts.Client()

		p.Notify(ctx, []snowflake.ID{user.ID}, Message{Title: "Trip", Body: "alice added Dinner", URL: "/"})
		p.Wait()

		Expect(reqs).To(HaveLen(2))
		for i, r := range reqs {
			Expect(r.Header.Get("Content-Encoding")).To(Equal("aes128gcm"))
			Expect(r.Header.Get("TTL")).To(Equal("86400"))
			Expect(r.Header.Get("Authorization")).To(HavePrefix("vapid t="))
			Expect(r.Header.Get("Authorization")).To(ContainSubstring(", k=" + p.PublicKey()))
			Expect(bodies[i]).To(BeNumerically(">", 0))
		}

		subs, err := model.ListPushSubscriptions(ctx, db, []snowflake.ID{user.ID})
		Expect(err).NotTo(HaveOccurred())
		Expect(subs).To(HaveLen(1))
		Expect(subs[0].Endpoint).To(Equal(ts.URL + "/ok"))
	})
})
