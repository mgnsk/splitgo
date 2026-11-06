package handler

import (
	"net/http"

	"github.com/mgnsk/splitgo"
	"github.com/mgnsk/splitgo/contract"
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/html"
	"github.com/mgnsk/splitgo/model"
	"github.com/mgnsk/splitgo/server"
	"github.com/mgnsk/webkit/snowflake"
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
	h.handle(mux, "POST /theme", h.SetTheme, true)
	h.handle(mux, "POST /timezone", h.SetTimezone, true)
}

// Home renders the overview of balances, friends and groups.
func (h *HomeHandler) Home(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)

	groups, err := model.ListUserGroups(ctx, h.DB, me.ID)
	if err != nil {
		panic(err)
	}

	friends, err := model.ListFriends(ctx, h.DB, me.ID)
	if err != nil {
		panic(err)
	}

	debts, err := model.UserDebts(ctx, h.DB, me.ID, groups)
	if err != nil {
		panic(err)
	}

	balances := domain.Balances(me.ID, debts)
	owed, owes := domain.Totals(balances)

	// Net per friend over all groups and non-group expenses.
	friendNets := map[snowflake.ID]map[string]int64{}
	for _, f := range friends {
		friendNets[f.User.ID] = map[string]int64{}
	}
	var others []domain.Balance
	for _, b := range balances {
		if net, ok := friendNets[b.UserID]; ok {
			net[b.Currency] += b.Amount
		} else {
			others = append(others, b)
		}
	}

	// Net per ordinary group.
	groupNets := map[snowflake.ID]map[string]int64{}
	for _, d := range debts {
		if groupNets[d.GroupID] == nil {
			groupNets[d.GroupID] = map[string]int64{}
		}
		if d.To == me.ID {
			groupNets[d.GroupID][d.Currency] += d.Amount
		} else {
			groupNets[d.GroupID][d.Currency] -= d.Amount
		}
	}

	var summaries []html.GroupSummary
	for _, g := range ordinaryGroups(groups) {
		summaries = append(summaries, html.GroupSummary{Group: g, Net: groupNets[g.ID]})
	}

	people := html.People{me.ID: me}
	var ids []snowflake.ID
	for _, b := range others {
		ids = append(ids, b.UserID)
	}
	h.addPeople(ctx, people, ids)

	server.RenderPage(w, r, h.SM, "Home", html.HomeMain(html.HomeData{
		Me:      me,
		Owed:    owed,
		Owes:    owes,
		Friends: friendItems(friends, friendNets),
		Others:  others,
		Groups:  summaries,
		People:  people,
	}))
}

func friendItems(friends []model.Friend, nets map[snowflake.ID]map[string]int64) []html.FriendItem {
	items := make([]html.FriendItem, len(friends))
	for i, f := range friends {
		items[i] = html.FriendItem{User: f.User, DirectGroupID: f.DirectGroupID, Net: nets[f.User.ID]}
	}
	return items
}

// Activity renders recent expenses across all of the user's groups and
// friends.
func (h *HomeHandler) Activity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)

	groups, err := model.ListUserGroups(ctx, h.DB, me.ID)
	if err != nil {
		panic(err)
	}

	expenses, err := model.ListUserExpenses(ctx, h.DB, me.ID, nil, activityLimit)
	if err != nil {
		panic(err)
	}

	people := html.People{me.ID: me}
	h.addPeople(ctx, people, expensePeopleIDs(expenses...))

	server.RenderPage(w, r, h.SM, "Activity", html.ActivityMain(me, expenses, h.ledgerLabels(ctx, me.ID, groups), people))
}

// Account renders and updates the account settings.
func (h *HomeHandler) Account(w http.ResponseWriter, r *http.Request) {
	me := mustUser(r)

	if r.Method == http.MethodGet {
		form := contract.AccountForm{Name: me.Name, Currency: me.Currency, Theme: string(me.Theme)}
		server.RenderPage(w, r, h.SM, "Account", html.AccountMain(me, form, nil, h.Push.PublicKey()))
		return
	}

	req := decode[contract.AccountForm](r)
	if errs := req.Validate(); len(errs) > 0 {
		server.RenderPage(w, r, h.SM, "Account", html.AccountMain(me, *req, errs, h.Push.PublicKey()))
		return
	}

	updated := *me
	updated.Name = req.Name
	updated.Currency = req.Currency
	updated.Theme = domain.Theme(req.Theme)

	if err := model.UpdateUser(r.Context(), h.DB, &updated); err != nil {
		panic(err)
	}

	server.Flash(r, h.SM, "Account saved")
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

// SetTimezone saves the time zone detected by the browser. Dates such as
// "today" for new expenses follow it.
func (h *HomeHandler) SetTimezone(w http.ResponseWriter, r *http.Request) {
	me := mustUser(r)
	tz := r.PostFormValue("timezone")

	if !domain.ValidTimezone(tz) {
		panic(splitgo.InvalidValue.New("Unknown time zone"))
	}

	if tz != me.Timezone {
		if err := model.SetUserTimezone(r.Context(), h.DB, me.ID, tz); err != nil {
			panic(err)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// SetTheme saves the color theme chosen with the top menu toggle and returns
// to the current page.
func (h *HomeHandler) SetTheme(w http.ResponseWriter, r *http.Request) {
	me := mustUser(r)
	req := decode[contract.ThemeForm](r)

	if errs := req.Validate(); len(errs) > 0 {
		panic(splitgo.InvalidValue.New(errs.Get("theme")))
	}

	updated := *me
	updated.Theme = domain.Theme(req.Theme)

	if err := model.UpdateUser(r.Context(), h.DB, &updated); err != nil {
		panic(err)
	}

	http.Redirect(w, r, safeNext(req.Next), http.StatusSeeOther)
}
