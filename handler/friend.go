package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	"github.com/mgnsk/splitgo/contract"
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/html"
	"github.com/mgnsk/splitgo/live"
	"github.com/mgnsk/splitgo/mail"
	"github.com/mgnsk/splitgo/model"
	"github.com/mgnsk/splitgo/server"
	"github.com/mgnsk/webkit"
	"github.com/mgnsk/webkit/push"
	"github.com/mgnsk/webkit/session"
	"github.com/mgnsk/webkit/snowflake"
	"github.com/mgnsk/webkit/web"
)

// FriendHandler handles friends and their non-group expenses.
type FriendHandler struct {
	*Deps
}

// NewFriendHandler creates a new friend handler.
func NewFriendHandler(deps *Deps) *FriendHandler {
	return &FriendHandler{Deps: deps}
}

// Register the handler.
func (h *FriendHandler) Register(mux *http.ServeMux) {
	h.handle(mux, "GET /friends/new", h.New, true)
	h.handle(mux, "POST /friends", h.Create, true)
	h.handle(mux, "GET /friends/{user_id}", h.Show, true)
	h.handle(mux, "POST /friends/{user_id}/remove", h.Remove, true)
	h.handle(mux, "GET /expenses/new", h.ChooseLedger, true)
}

// New renders the add friend form.
func (h *FriendHandler) New(w http.ResponseWriter, r *http.Request) {
	server.RenderPage(w, r, h.SM, "Add friend", html.AddFriendMain("", nil))
}

// Create adds a friend by email, creating the user if needed, and emails
// them a sign-in link.
func (h *FriendHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)
	req := web.Decode[contract.FriendForm](r)

	errs := req.Validate()
	email := domain.NormalizeEmail(req.Email)
	if len(errs) == 0 && email == domain.NormalizeEmail(me.Email) {
		errs = url.Values{"email": {"That's your own email address"}}
	}
	if len(errs) > 0 {
		server.RenderPage(w, r, h.SM, "Add friend", html.AddFriendMain(req.Email, errs))
		return
	}

	alreadyFriends := func(friend *domain.User) {
		session.Flash(r.Context(), h.SM, fmt.Sprintf("%s is already your friend", friend.DisplayName()))
		http.Redirect(w, r, html.FriendURL(friend.ID), http.StatusSeeOther)
	}

	friend, err := model.GetUserByEmail(ctx, h.DB, email)
	switch {
	case err == nil:
		if h.directGroup(ctx, me.ID, friend.ID) != nil {
			alreadyFriends(friend)
			return
		}
	case !errors.Is(err, webkit.NotFound):
		panic(err)
	}

	if !h.canInvite(ctx, me) {
		server.RenderPage(w, r, h.SM, "Add friend", html.AddFriendMain(req.Email, url.Values{"email": {inviteLimitMessage}}))
		return
	}

	if friend == nil {
		friend = &domain.User{ID: snowflake.Generate(), Email: email, Currency: me.Currency}
		if err := model.InsertUser(ctx, h.DB, friend); err != nil {
			panic(err)
		}
	}

	group, created, err := model.GetOrCreateDirectGroup(ctx, h.DB, me.ID, friend.ID, me.Currency)
	if err != nil {
		panic(err)
	}

	if !created {
		alreadyFriends(friend)
		return
	}

	link, _, err := h.createLogin(ctx, email, html.FriendURL(me.ID), InviteTokenTTL, false)
	if err != nil {
		panic(err)
	}

	if err := h.Mailer.Send(ctx, mail.Message{
		To:      email,
		Subject: fmt.Sprintf("%s added you as a friend on splitgo", me.DisplayName()),
		Body: fmt.Sprintf("Hi,\n\n%s added you as a friend on splitgo to share expenses.\n\n"+
			"Sign in to see it:\n\n%s\n\nThe link expires in 7 days. "+
			"You can always get a new link by signing in with this email address at %s/login\n",
			me.DisplayName(), link, h.Config.BaseURL),
	}); err != nil {
		panic(err)
	}
	h.recordInvite(ctx, me)

	h.Notifier.Notify(ctx, []snowflake.ID{friend.ID}, push.Message{
		Title: "splitgo",
		Body:  fmt.Sprintf("%s added you as a friend", me.DisplayName()),
		URL:   html.FriendURL(me.ID),
	})
	h.Live.Publish([]snowflake.ID{friend.ID}, live.Event{GroupID: group.ID})

	session.Flash(r.Context(), h.SM, "Friend added")
	http.Redirect(w, r, html.FriendURL(friend.ID), http.StatusSeeOther)
}

// Show renders the balance and shared expenses with a person: a friend, or
// someone the user shares a group or expenses with.
func (h *FriendHandler) Show(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)
	req := web.Decode[contract.FriendPath](r)
	friendID := req.UserID

	if friendID == me.ID {
		panic(webkit.NotFound.New("Friend not found"))
	}

	direct := h.directGroup(ctx, me.ID, friendID)

	expenses, err := model.ListUserExpensesWith(ctx, h.DB, me.ID, friendID, nil, activityLimit)
	if err != nil {
		panic(err)
	}

	if direct == nil && len(expenses) == 0 {
		shares, err := model.SharesGroup(ctx, h.DB, me.ID, friendID)
		if err != nil {
			panic(err)
		}
		if !shares {
			panic(webkit.NotFound.New("Friend not found"))
		}
	}

	groups, err := model.ListUserGroups(ctx, h.DB, me.ID)
	if err != nil {
		panic(err)
	}

	debts, err := model.UserDebts(ctx, h.DB, me.ID, groups)
	if err != nil {
		panic(err)
	}

	var (
		balances []domain.Balance
		between  []domain.Debt
	)
	for _, b := range domain.Balances(me.ID, debts) {
		if b.UserID == friendID {
			balances = append(balances, b)
		}
	}
	for _, d := range debts {
		if d.From == friendID || d.To == friendID {
			between = append(between, d)
		}
	}

	people := html.People{me.ID: me}
	h.addPeople(ctx, people, append(expensePeopleIDs(expenses...), friendID))

	friend, ok := people[friendID]
	if !ok {
		panic(webkit.NotFound.New("Friend not found"))
	}

	labels := h.ledgerLabels(ctx, me.ID, groups)

	data := html.FriendData{
		Me:       me,
		Friend:   friend,
		Balances: balances,
		Debts:    between,
		Labels:   labels,
		Expenses: expenses,
		People:   people,
	}
	if direct != nil {
		data.DirectGroupID = direct.ID
		data.CanRemove = !slices.ContainsFunc(debts, func(d domain.Debt) bool { return d.GroupID == direct.ID })
	}

	server.RenderPage(w, r, h.SM, friend.DisplayName(), html.FriendMain(data))
}

// Remove removes a friend whose non-group expenses are settled, deleting
// those expenses. Shared groups are not affected.
func (h *FriendHandler) Remove(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)
	req := web.Decode[contract.FriendPath](r)

	direct := h.directGroup(ctx, me.ID, req.UserID)
	if direct == nil {
		panic(webkit.NotFound.New("Friend not found"))
	}

	debts, err := model.GroupDebts(ctx, h.DB, direct)
	if err != nil {
		panic(err)
	}
	if len(debts) > 0 {
		panic(webkit.PreconditionFailed.New("Settle up before removing the friend"))
	}

	if err := model.DeleteGroup(ctx, h.DB, direct.ID); err != nil {
		panic(err)
	}

	h.Live.Publish([]snowflake.ID{req.UserID}, live.Event{GroupID: direct.ID})

	session.Flash(r.Context(), h.SM, "Friend removed")
	web.Redirect(w, r, "/")
}

// ChooseLedger renders the list of friends and groups to add an expense
// with.
func (h *FriendHandler) ChooseLedger(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)

	friends, err := model.ListFriends(ctx, h.DB, me.ID)
	if err != nil {
		panic(err)
	}

	groups, err := model.ListUserGroups(ctx, h.DB, me.ID)
	if err != nil {
		panic(err)
	}

	server.RenderPage(w, r, h.SM, "Add expense", html.ChooseLedgerMain(friendItems(friends, nil), ordinaryGroups(groups)))
}

// inviteLimitMessage tells the user they sent too many invite emails.
var inviteLimitMessage = fmt.Sprintf("You can send up to %d invites a day. Try again later.", model.MaxInvites)

// canInvite reports whether the user may send another invite email.
func (d *Deps) canInvite(ctx context.Context, me *domain.User) bool {
	ok, err := model.CanInvite(ctx, d.DB, me.ID, d.now())
	if err != nil {
		panic(err)
	}
	return ok
}

// recordInvite counts an invite email sent by the user.
func (d *Deps) recordInvite(ctx context.Context, me *domain.User) {
	if err := model.InsertInvite(ctx, d.DB, me.ID, d.now()); err != nil {
		panic(err)
	}
}

// directGroup returns the direct group of two users, or nil if they are
// not friends.
func (d *Deps) directGroup(ctx context.Context, a, b snowflake.ID) *domain.Group {
	g, err := model.GetDirectGroup(ctx, d.DB, a, b)
	if errors.Is(err, webkit.NotFound) {
		return nil
	}
	if err != nil {
		panic(err)
	}
	return g
}

// ledgerLabels names the user's groups for expense lists: ordinary groups by
// name and non-group expenses by the friend's name.
func (d *Deps) ledgerLabels(ctx context.Context, userID snowflake.ID, groups []*domain.Group) map[snowflake.ID]string {
	labels := map[snowflake.ID]string{}
	for _, g := range groups {
		if !g.Direct() {
			labels[g.ID] = g.Name
		}
	}

	friends, err := model.ListFriends(ctx, d.DB, userID)
	if err != nil {
		panic(err)
	}
	for _, f := range friends {
		labels[f.DirectGroupID] = f.User.DisplayName()
	}

	return labels
}

func ordinaryGroups(groups []*domain.Group) []*domain.Group {
	var result []*domain.Group
	for _, g := range groups {
		if !g.Direct() {
			result = append(result, g)
		}
	}
	return result
}
