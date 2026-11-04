package handler

import (
	"net/http"

	"github.com/mgnsk/splitgo"
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
	h.handle(mux, "POST /theme", h.SetTheme, true)
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

// Activity renders recent expenses across all of the user's groups, or
// those shared with one friend when the with query parameter is set.
func (h *HomeHandler) Activity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)

	groups, err := model.ListUserGroups(ctx, h.DB, me.ID)
	if err != nil {
		panic(err)
	}

	if q := r.URL.Query().Get("with"); q != "" {
		friendID, ok := parseFormID(q)
		if !ok || friendID == me.ID {
			panic(splitgo.NotFound.New("Friend not found"))
		}
		h.friendActivity(w, r, me, friendID, groups)
		return
	}

	expenses, err := model.ListUserExpenses(ctx, h.DB, me.ID, nil, activityLimit)
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

func (h *HomeHandler) friendActivity(w http.ResponseWriter, r *http.Request, me *domain.User, friendID snowflake.ID, groups []*domain.Group) {
	ctx := r.Context()

	expenses, err := model.ListUserExpensesWith(ctx, h.DB, me.ID, friendID, nil, activityLimit)
	if err != nil {
		panic(err)
	}

	// Only show people the user shares a group or expenses with.
	if len(expenses) == 0 {
		shares, err := model.SharesGroup(ctx, h.DB, me.ID, friendID)
		if err != nil {
			panic(err)
		}
		if !shares {
			panic(splitgo.NotFound.New("Friend not found"))
		}
	}

	debts, err := model.UserDebts(ctx, h.DB, me.ID, groups)
	if err != nil {
		panic(err)
	}

	var balances []domain.Balance
	for _, b := range domain.Balances(me.ID, debts) {
		if b.UserID == friendID {
			balances = append(balances, b)
		}
	}

	groupsByID := map[snowflake.ID]*domain.Group{}
	for _, g := range groups {
		groupsByID[g.ID] = g
	}

	people := html.People{me.ID: me}
	h.addPeople(ctx, people, append(expensePeopleIDs(expenses...), friendID))

	friend, ok := people[friendID]
	if !ok {
		panic(splitgo.NotFound.New("Friend not found"))
	}

	server.RenderPage(w, r, h.SM, friend.DisplayName(), html.FriendActivityMain(me, friend, balances, expenses, groupsByID, people))
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
