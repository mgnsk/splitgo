// Package push sends Web Push notifications to users' devices.
package push

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/mgnsk/splitgo/model"
	"github.com/mgnsk/webkit/snowflake"
	"github.com/uptrace/bun"
)

// Message is a notification shown on the user's device.
type Message struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	// URL is the local path opened when the notification is tapped.
	URL string `json:"url"`
	// Tag makes a newer notification with the same tag replace an older one.
	Tag string `json:"tag,omitempty"`
}

// Notifier notifies users.
type Notifier interface {
	// Notify sends msg to all devices of users. Delivery happens in the
	// background and failures are only logged.
	Notify(ctx context.Context, userIDs []snowflake.ID, msg Message)
	// PublicKey returns the VAPID public key browsers subscribe with.
	PublicKey() string
}

// pushHosts are the push services the server delivers to. Subscription
// endpoints are supplied by clients, so other hosts are rejected to keep the
// server from making requests anywhere else.
var pushHosts = []string{
	"push.apple.com",            // Safari, iOS
	"fcm.googleapis.com",        // Chrome, Edge on Android
	"push.services.mozilla.com", // Firefox
	"notify.windows.com",        // Edge on Windows
}

// ValidEndpoint reports whether endpoint is an HTTPS URL of a known push
// service.
func ValidEndpoint(endpoint string) bool {
	if len(endpoint) > 1024 {
		return false
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, h := range pushHosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}

// WebPush sends notifications with the Web Push protocol.
type WebPush struct {
	db         *bun.DB
	keys       *model.VAPIDKeys
	subscriber string
	client     *http.Client
	wg         sync.WaitGroup
}

// New creates a Web Push notifier, generating the VAPID keys on first use.
// subscriber is a contact email address or https URL sent to push services.
func New(ctx context.Context, db *bun.DB, subscriber string) (*WebPush, error) {
	keys, err := model.GetOrCreateVAPIDKeys(ctx, db, webpush.GenerateVAPIDKeys)
	if err != nil {
		return nil, err
	}

	return &WebPush{
		db:         db,
		keys:       keys,
		subscriber: subscriber,
		client:     &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// PublicKey returns the VAPID public key.
func (p *WebPush) PublicKey() string {
	return p.keys.PublicKey
}

// Notify sends msg to all devices of users in the background.
func (p *WebPush) Notify(ctx context.Context, userIDs []snowflake.ID, msg Message) {
	if len(userIDs) == 0 {
		return
	}

	subs, err := model.ListPushSubscriptions(ctx, p.db, userIDs)
	if err != nil {
		slog.Error("error listing push subscriptions", slog.String("error", err.Error()))
		return
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}

	for _, s := range subs {
		// SendNotification appends padding to the message, writing into its
		// spare capacity, so each send needs its own copy.
		p.wg.Go(func() { p.send(s, slices.Clone(payload)) })
	}
}

// Wait waits for notifications being sent.
func (p *WebPush) Wait() {
	p.wg.Wait()
}

func (p *WebPush) send(s *model.PushSubscription, payload []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	log := slog.With(slog.String("push_host", hostOf(s.Endpoint)), slog.String("user_id", s.UserID.String()))

	resp, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: s.Endpoint,
		Keys:     webpush.Keys{P256dh: s.P256dh, Auth: s.Auth},
	}, &webpush.Options{
		HTTPClient:      p.client,
		Subscriber:      p.subscriber,
		VAPIDPublicKey:  p.keys.PublicKey,
		VAPIDPrivateKey: p.keys.PrivateKey,
		TTL:             int((24 * time.Hour).Seconds()),
		Urgency:         webpush.UrgencyNormal,
	})
	if err != nil {
		log.Warn("error sending push notification", slog.String("error", err.Error()))
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))

	switch {
	case resp.StatusCode < 300:
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		// The user revoked permission or the subscription expired.
		if err := model.DeletePushSubscription(ctx, p.db, s.Endpoint); err != nil {
			log.Error("error deleting push subscription", slog.String("error", err.Error()))
		}
	default:
		log.Warn("push service rejected notification",
			slog.Int("status", resp.StatusCode),
			slog.String("body", strings.TrimSpace(string(body))),
		)
	}
}

func hostOf(endpoint string) string {
	if u, err := url.Parse(endpoint); err == nil {
		return u.Host
	}
	return ""
}
