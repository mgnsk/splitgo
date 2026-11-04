package handler

import (
	"context"
	"net/http"
	"slices"

	"github.com/mgnsk/splitgo/contract"
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/html"
	"github.com/mgnsk/splitgo/model"
	"github.com/mgnsk/splitgo/pkg/snowflake"
	"github.com/mgnsk/splitgo/server"
	"github.com/mgnsk/splitgo/split"
)

// SettleHandler records payments between group members.
type SettleHandler struct {
	*Deps
}

// NewSettleHandler creates a new settle handler.
func NewSettleHandler(deps *Deps) *SettleHandler {
	return &SettleHandler{Deps: deps}
}

// Register the handler.
func (h *SettleHandler) Register(mux *http.ServeMux) {
	h.handle(mux, "GET /groups/{group_id}/settle", h.Form, true)
	h.handle(mux, "POST /groups/{group_id}/settle", h.Settle, true)
}

// Form renders the record payment form, prefilled from query parameters.
func (h *SettleHandler) Form(w http.ResponseWriter, r *http.Request) {
	me := mustUser(r)
	req := decode[contract.GroupPath](r)
	d := h.loadGroup(r.Context(), me, req.GroupID)
	parties := h.settleParties(r.Context(), d)
	q := r.URL.Query()

	form := contract.SettleForm{
		From:     me.ID,
		Amount:   q.Get("amount"),
		Currency: d.Group.Currency,
		Date:     today(h.userNow(me)),
	}

	ids := memberIDs(parties)
	if id, ok := parseFormID(q.Get("from")); ok && slices.Contains(ids, id) {
		form.From = id
	}
	if id, ok := parseFormID(q.Get("to")); ok && slices.Contains(ids, id) {
		form.To = id
	} else {
		for _, m := range parties {
			if m.ID != form.From {
				form.To = m.ID
				break
			}
		}
	}
	if c := q.Get("currency"); c != "" {
		form.Currency = c
	}

	server.RenderPage(w, r, h.SM, "Record a payment", html.SettleMain(d, parties, form, nil))
}

// settleParties returns the people who can pay or receive in the group: the
// members, followed by former members who still have open debts there, e.g.
// when debt simplification was turned off after they left.
func (h *SettleHandler) settleParties(ctx context.Context, d html.GroupData) []*domain.User {
	debts, err := model.GroupDebts(ctx, h.DB, d.Group)
	if err != nil {
		panic(err)
	}

	var ids []snowflake.ID
	for _, debt := range debts {
		ids = append(ids, debt.From, debt.To)
	}
	h.addPeople(ctx, d.People, ids)

	parties := slices.Clone(d.Members)
	seen := map[snowflake.ID]bool{}
	for _, m := range d.Members {
		seen[m.ID] = true
	}
	for _, id := range ids {
		if u, ok := d.People[id]; ok && !seen[id] {
			seen[id] = true
			parties = append(parties, u)
		}
	}
	return parties
}

// Settle records a payment.
func (h *SettleHandler) Settle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)
	req := decode[contract.SettleForm](r)
	d := h.loadGroup(ctx, me, req.GroupID)

	parties := h.settleParties(ctx, d)

	resolved, errs := req.Validate(memberIDs(parties))
	if len(errs) > 0 {
		server.RenderPage(w, r, h.SM, "Record a payment", html.SettleMain(d, parties, *req, errs))
		return
	}

	// A payment from A to B is an expense paid by A where B owes the full
	// amount, which cancels A's debt to B.
	e := &domain.Expense{
		ID:           snowflake.Generate(),
		GroupID:      d.Group.ID,
		Kind:         domain.KindSettlement,
		Description:  "Payment",
		Amount:       resolved.Amount,
		Currency:     resolved.Currency.Code,
		SplitType:    split.Exact,
		PaidBy:       resolved.From,
		AddedBy:      me.ID,
		Date:         resolved.Date,
		UpdatedAt:    h.now(),
		Participants: []domain.Participant{{UserID: resolved.To, Share: resolved.Amount}},
	}

	if err := model.InsertExpense(ctx, h.DB, e); err != nil {
		panic(err)
	}

	h.notifySettlement(r, d, e, resolved.To)

	h.publishGroup(d.Group.ID, d.Members)

	server.Flash(r, h.SM, "Payment recorded")
	http.Redirect(w, r, d.LedgerURL(), http.StatusSeeOther)
}
