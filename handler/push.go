package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mgnsk/splitgo"
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/html"
	"github.com/mgnsk/splitgo/model"
	"github.com/mgnsk/splitgo/pkg/snowflake"
	"github.com/mgnsk/splitgo/push"
	"github.com/samber/lo"
)

// PushHandler manages the push subscriptions of the user's devices.
type PushHandler struct {
	*Deps
}

// NewPushHandler creates a new push handler.
func NewPushHandler(deps *Deps) *PushHandler {
	return &PushHandler{Deps: deps}
}

// Register the handler.
func (h *PushHandler) Register(mux *http.ServeMux) {
	h.handle(mux, "GET /push/key", h.Key, true)
	h.handle(mux, "POST /push/subscribe", h.Subscribe, true)
	h.handle(mux, "POST /push/unsubscribe", h.Unsubscribe, true)
}

// subscriptionJSON is a PushSubscription serialized by the browser.
type subscriptionJSON struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

func decodeSubscription(r *http.Request) subscriptionJSON {
	var s subscriptionJSON
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4096)).Decode(&s); err != nil {
		panic(splitgo.InvalidValue.New("Invalid subscription", err))
	}
	if !push.ValidEndpoint(s.Endpoint) {
		panic(splitgo.InvalidValue.New("Unsupported push service"))
	}
	return s
}

// Key returns the VAPID public key to subscribe with.
func (h *PushHandler) Key(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(h.Push.PublicKey()))
}

// Subscribe saves the push subscription of the current device.
func (h *PushHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
	me := mustUser(r)
	s := decodeSubscription(r)

	if s.Keys.P256dh == "" || s.Keys.Auth == "" || len(s.Keys.P256dh) > 256 || len(s.Keys.Auth) > 256 {
		panic(splitgo.InvalidValue.New("Invalid subscription keys"))
	}

	if err := model.SavePushSubscription(r.Context(), h.DB, &model.PushSubscription{
		Endpoint: s.Endpoint,
		UserID:   me.ID,
		P256dh:   s.Keys.P256dh,
		Auth:     s.Keys.Auth,
	}, h.now()); err != nil {
		panic(err)
	}

	w.WriteHeader(http.StatusNoContent)
}

// Unsubscribe deletes the push subscription of the current device.
func (h *PushHandler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	me := mustUser(r)
	s := decodeSubscription(r)

	if err := model.DeleteUserPushSubscription(r.Context(), h.DB, me.ID, s.Endpoint); err != nil {
		panic(err)
	}

	w.WriteHeader(http.StatusNoContent)
}

// notifyExpense notifies the payer and participants of a new expense.
func (d *Deps) notifyExpense(r *http.Request, data html.GroupData, e *domain.Expense) {
	me := data.Me
	path := groupPath(data.Group.ID) + "/expenses/" + e.ID.String()
	total := html.Fmt(e.Amount, e.Currency)

	ids := append([]snowflake.ID{e.PaidBy}, e.ParticipantIDs()...)
	for _, id := range lo.Uniq(lo.Without(ids, me.ID)) {
		body := fmt.Sprintf("%s added %q: %s", me.DisplayName(), e.Description, total)
		if share := e.ShareOf(id); share > 0 {
			body += fmt.Sprintf(", your share %s", html.Fmt(share, e.Currency))
		}
		d.Push.Notify(r.Context(), []snowflake.ID{id}, push.Message{
			Title: data.Group.Name,
			Body:  body,
			URL:   path,
			Tag:   e.ID.String(),
		})
	}
}

// notifySettlement notifies the payer and recipient of a recorded payment.
func (d *Deps) notifySettlement(r *http.Request, data html.GroupData, e *domain.Expense, to snowflake.ID) {
	me := data.Me
	amount := html.Fmt(e.Amount, e.Currency)

	for _, id := range lo.Uniq(lo.Without([]snowflake.ID{e.PaidBy, to}, me.ID)) {
		var body string
		switch {
		case id == to && me.ID == e.PaidBy:
			body = fmt.Sprintf("%s paid you %s", me.DisplayName(), amount)
		case id == to:
			body = fmt.Sprintf("%s recorded that %s paid you %s", me.DisplayName(), data.People.Name(e.PaidBy, me.ID), amount)
		case me.ID == to:
			body = fmt.Sprintf("%s recorded your payment of %s", me.DisplayName(), amount)
		default:
			body = fmt.Sprintf("%s recorded that you paid %s %s", me.DisplayName(), data.People.Name(to, me.ID), amount)
		}

		d.Push.Notify(r.Context(), []snowflake.ID{id}, push.Message{
			Title: data.Group.Name,
			Body:  body,
			URL:   groupPath(data.Group.ID),
			Tag:   e.ID.String(),
		})
	}
}
