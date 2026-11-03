package handler

import (
	"net/http"

	"github.com/mgnsk/splitgo/contract"
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/html"
	"github.com/mgnsk/splitgo/model"
	"github.com/mgnsk/splitgo/pkg/snowflake"
	"github.com/mgnsk/splitgo/server"
)

const activityLimit = 50

// HomeHandler handles the overview, activity and account pages.
type HomeHandler struct {
	*Deps
}

// NewHomeHandler creates a new home handler.
func NewHomeHandler(deps *Deps) *HomeHandler {
	return &HomeHandler{Deps: deps}
}

// Register the handler.
func (h *HomeHandler) Register(mux *http.ServeMux) {
	h.handle(mux, "GET /{$}", h.Home, true)
	h.handle(mux, "GET /activity", h.Activity, true)
	h.handle(mux, "GET /account", h.Account, true)
	h.handle(mux, "POST /account", h.Account, true)
}

// Home renders the overview of balances and groups.
func (h *HomeHandler) Home(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)

	groups, err := model.ListUserGroups(ctx, h.DB, me.ID)
	if err != nil {
		panic(err)
	}

	debts, err := model.UserDebts(ctx, h.DB, me.ID, groups)
	if err != nil {
		panic(err)
	}

	balances := domain.Balances(me.ID, debts)
	owed, owes := domain.Totals(balances)

	nets := map[snowflake.ID]map[string]int64{}
	for _, d := range debts {
		if nets[d.GroupID] == nil {
			nets[d.GroupID] = map[string]int64{}
		}
		if d.To == me.ID {
			nets[d.GroupID][d.Currency] += d.Amount
		} else {
			nets[d.GroupID][d.Currency] -= d.Amount
		}
	}

	summaries := make([]html.GroupSummary, len(groups))
	for i, g := range groups {
		summaries[i] = html.GroupSummary{Group: g, Net: nets[g.ID]}
	}

	people := html.People{me.ID: me}
	var ids []snowflake.ID
	for _, b := range balances {
		ids = append(ids, b.UserID)
	}
	h.addPeople(ctx, people, ids)

	server.RenderPage(w, r, h.SM, "Home", html.HomeMain(html.HomeData{
		Me:       me,
		Owed:     owed,
		Owes:     owes,
		Balances: balances,
		Groups:   summaries,
		People:   people,
	}))
}

// Activity renders recent expenses across all of the user's groups.
func (h *HomeHandler) Activity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)

	expenses, err := model.ListUserExpenses(ctx, h.DB, me.ID, nil, activityLimit)
	if err != nil {
		panic(err)
	}

	groups, err := model.ListUserGroups(ctx, h.DB, me.ID)
	if err != nil {
		panic(err)
	}

	groupsByID := map[snowflake.ID]*domain.Group{}
	for _, g := range groups {
		groupsByID[g.ID] = g
	}

	people := html.People{me.ID: me}
	h.addPeople(ctx, people, expensePeopleIDs(expenses...))

	server.RenderPage(w, r, h.SM, "Activity", html.ActivityMain(me, expenses, groupsByID, people))
}

// Account renders and updates the account settings.
func (h *HomeHandler) Account(w http.ResponseWriter, r *http.Request) {
	me := mustUser(r)

	if r.Method == http.MethodGet {
		form := contract.AccountForm{Name: me.Name, Currency: me.Currency}
		server.RenderPage(w, r, h.SM, "Account", html.AccountMain(me, form, nil))
		return
	}

	req := decode[contract.AccountForm](r)
	if errs := req.Validate(); len(errs) > 0 {
		server.RenderPage(w, r, h.SM, "Account", html.AccountMain(me, *req, errs))
		return
	}

	updated := *me
	updated.Name = req.Name
	updated.Currency = req.Currency

	if err := model.UpdateUser(r.Context(), h.DB, &updated); err != nil {
		panic(err)
	}

	server.Flash(r, h.SM, "Account saved")
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}
