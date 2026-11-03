package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/mgnsk/splitgo"
	"github.com/mgnsk/splitgo/contract"
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/html"
	"github.com/mgnsk/splitgo/mail"
	"github.com/mgnsk/splitgo/model"
	"github.com/mgnsk/splitgo/pkg/snowflake"
	"github.com/mgnsk/splitgo/server"
	hxhttp "maragu.dev/gomponents-htmx/http"
)

const expensePageSize = 30

// GroupHandler handles groups and membership.
type GroupHandler struct {
	*Deps
}

// NewGroupHandler creates a new group handler.
func NewGroupHandler(deps *Deps) *GroupHandler {
	return &GroupHandler{Deps: deps}
}

// Register the handler.
func (h *GroupHandler) Register(mux *http.ServeMux) {
	h.handle(mux, "GET /groups/new", h.NewGroup, true)
	h.handle(mux, "POST /groups", h.CreateGroup, true)
	h.handle(mux, "GET /groups/{group_id}", h.Show, true)
	h.handle(mux, "GET /groups/{group_id}/settings", h.Settings, true)
	h.handle(mux, "POST /groups/{group_id}/settings", h.UpdateSettings, true)
	h.handle(mux, "POST /groups/{group_id}/members", h.AddMember, true)
	h.handle(mux, "POST /groups/{group_id}/leave", h.Leave, true)
	h.handle(mux, "POST /groups/{group_id}/delete", h.Delete, true)
	h.handle(mux, "GET /join/{token}", h.JoinPage, true)
	h.handle(mux, "POST /join/{token}", h.Join, true)
}

// NewGroup renders the create group form.
func (h *GroupHandler) NewGroup(w http.ResponseWriter, r *http.Request) {
	me := mustUser(r)
	server.RenderPage(w, r, h.SM, "New group", html.NewGroupMain(contract.GroupForm{Currency: me.Currency}, nil))
}

// CreateGroup creates a group.
func (h *GroupHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	me := mustUser(r)
	req := decode[contract.GroupForm](r)

	if errs := req.Validate(); len(errs) > 0 {
		server.RenderPage(w, r, h.SM, "New group", html.NewGroupMain(*req, errs))
		return
	}

	group := &domain.Group{
		ID:         snowflake.Generate(),
		Name:       req.Name,
		Currency:   req.Currency,
		InviteCode: randomToken(16),
		CreatedBy:  me.ID,
	}

	if err := model.InsertGroup(r.Context(), h.DB, group); err != nil {
		panic(err)
	}

	server.Flash(r, h.SM, "Group created")
	http.Redirect(w, r, groupPath(group.ID), http.StatusSeeOther)
}

// Show renders the group balances and expenses. For htmx requests with a
// cursor it renders the next page of expense list items.
func (h *GroupHandler) Show(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)
	req := decode[contract.GroupPage](r)
	d := h.loadGroup(ctx, me, req.GroupID)

	var cursor *model.ExpenseCursor
	if req.AfterDate != "" && req.AfterID != 0 {
		cursor = &model.ExpenseCursor{Date: req.AfterDate, ID: req.AfterID}
	}

	expenses, err := model.ListGroupExpenses(ctx, h.DB, req.GroupID, cursor, expensePageSize+1)
	if err != nil {
		panic(err)
	}

	more := len(expenses) > expensePageSize
	if more {
		expenses = expenses[:expensePageSize]
	}

	h.addPeople(ctx, d.People, expensePeopleIDs(expenses...))

	if hxhttp.IsRequest(r.Header) && cursor != nil {
		server.RenderPartial(w, html.ExpenseListPartial(d, expenses, more))
		return
	}

	debts, err := model.GroupDebts(ctx, h.DB, d.Group)
	if err != nil {
		panic(err)
	}

	spending, err := model.GroupSpending(ctx, h.DB, d.Group.ID)
	if err != nil {
		panic(err)
	}

	var ids []snowflake.ID
	for _, debt := range debts {
		ids = append(ids, debt.From, debt.To)
	}
	h.addPeople(ctx, d.People, ids)

	server.RenderPage(w, r, h.SM, d.Group.Name, html.GroupMain(d, debts, spending, expenses, more))
}

// Settings renders the group settings.
func (h *GroupHandler) Settings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)
	req := decode[contract.GroupPath](r)
	d := h.loadGroup(ctx, me, req.GroupID)

	h.renderSettings(w, r, d, contract.GroupForm{
		Name:          d.Group.Name,
		Currency:      d.Group.Currency,
		SimplifyDebts: d.Group.SimplifyDebts,
	}, nil, "", nil)
}

func (h *GroupHandler) renderSettings(
	w http.ResponseWriter,
	r *http.Request,
	d html.GroupData,
	form contract.GroupForm,
	errs url.Values,
	addEmail string,
	addErrs url.Values,
) {
	settled, allSettled := h.settledState(r.Context(), d)

	server.RenderPage(w, r, h.SM, d.Group.Name, html.GroupSettingsMain(html.GroupSettingsData{
		GroupData: d,
		Form:      form,
		Errors:    errs,
		AddEmail:  addEmail,
		AddErrors: addErrs,
		InviteURL: fmt.Sprintf("%s/join/%s", h.Config.BaseURL, d.Group.InviteCode),
		CanLeave:  settled && len(d.Members) > 1,
		CanDelete: allSettled,
	}))
}

// settledState reports whether the user and the whole group are settled.
// Pairwise debts are used regardless of simplification so that nobody is
// left with an unrecoverable debt.
func (h *GroupHandler) settledState(ctx context.Context, d html.GroupData) (me, all bool) {
	pairwise := *d.Group
	pairwise.SimplifyDebts = false

	debts, err := model.GroupDebts(ctx, h.DB, &pairwise)
	if err != nil {
		panic(err)
	}

	me = !slices.ContainsFunc(debts, func(debt domain.Debt) bool {
		return debt.From == d.Me.ID || debt.To == d.Me.ID
	})

	return me, len(debts) == 0
}

// UpdateSettings updates the group settings.
func (h *GroupHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)
	req := decode[contract.GroupForm](r)
	d := h.loadGroup(ctx, me, req.GroupID)

	if errs := req.Validate(); len(errs) > 0 {
		h.renderSettings(w, r, d, *req, errs, "", nil)
		return
	}

	updated := *d.Group
	updated.Name = req.Name
	updated.Currency = req.Currency
	updated.SimplifyDebts = req.SimplifyDebts

	if err := model.UpdateGroup(ctx, h.DB, &updated); err != nil {
		panic(err)
	}

	server.Flash(r, h.SM, "Group saved")
	http.Redirect(w, r, groupPath(d.Group.ID)+"/settings", http.StatusSeeOther)
}

// AddMember adds a member by email, creating the user if needed, and emails
// them a sign-in link to the group.
func (h *GroupHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)
	req := decode[contract.AddMemberForm](r)
	d := h.loadGroup(ctx, me, req.GroupID)

	if errs := req.Validate(); len(errs) > 0 {
		h.renderSettings(w, r, d, groupForm(d.Group), nil, req.Email, errs)
		return
	}

	email := domain.NormalizeEmail(req.Email)

	user, err := model.GetUserByEmail(ctx, h.DB, email)
	if errors.Is(err, splitgo.NotFound) {
		user = &domain.User{ID: snowflake.Generate(), Email: email, Currency: d.Group.Currency}
		err = model.InsertUser(ctx, h.DB, user)
	}
	if err != nil {
		panic(err)
	}

	if slices.ContainsFunc(d.Members, func(m *domain.User) bool { return m.ID == user.ID }) {
		h.renderSettings(w, r, d, groupForm(d.Group), nil, req.Email, url.Values{"email": {"Already a member"}})
		return
	}

	if err := model.AddGroupMember(ctx, h.DB, d.Group.ID, user.ID); err != nil {
		panic(err)
	}

	link, err := h.createLoginLink(ctx, email, groupPath(d.Group.ID), InviteTokenTTL)
	if err != nil {
		panic(err)
	}

	if err := h.Mailer.Send(ctx, mail.Message{
		To:      email,
		Subject: fmt.Sprintf("%s added you to %s on splitgo", me.DisplayName(), d.Group.Name),
		Body: fmt.Sprintf("Hi,\n\n%s added you to the group \"%s\" on splitgo to share expenses.\n\n"+
			"Sign in to see it:\n\n%s\n\nThe link expires in 7 days. "+
			"You can always get a new link by signing in with this email address at %s/login\n",
			me.DisplayName(), d.Group.Name, link, h.Config.BaseURL),
	}); err != nil {
		panic(err)
	}

	server.Flash(r, h.SM, "Member added")
	http.Redirect(w, r, groupPath(d.Group.ID)+"/settings", http.StatusSeeOther)
}

// Leave removes the user from the group.
func (h *GroupHandler) Leave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)
	req := decode[contract.GroupPath](r)
	d := h.loadGroup(ctx, me, req.GroupID)

	if settled, _ := h.settledState(ctx, d); !settled {
		panic(splitgo.PreconditionFailed.New("Settle up your balances before leaving the group"))
	}

	if len(d.Members) == 1 {
		panic(splitgo.PreconditionFailed.New("You are the last member, delete the group instead"))
	}

	if err := model.RemoveGroupMember(ctx, h.DB, d.Group.ID, me.ID); err != nil {
		panic(err)
	}

	server.Flash(r, h.SM, fmt.Sprintf("You left %s", d.Group.Name))
	server.Redirect(w, r, "/")
}

// Delete deletes the group. Only the creator can delete a settled group.
func (h *GroupHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)
	req := decode[contract.GroupPath](r)
	d := h.loadGroup(ctx, me, req.GroupID)

	if d.Group.CreatedBy != me.ID {
		panic(splitgo.Forbidden.New("Only the creator can delete the group"))
	}

	if _, all := h.settledState(ctx, d); !all {
		panic(splitgo.PreconditionFailed.New("Everyone must be settled up before deleting the group"))
	}

	if err := model.DeleteGroup(ctx, h.DB, d.Group.ID); err != nil {
		panic(err)
	}

	server.Flash(r, h.SM, fmt.Sprintf("Deleted %s", d.Group.Name))
	server.Redirect(w, r, "/")
}

// JoinPage renders the join group confirmation.
func (h *GroupHandler) JoinPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)
	group, members := h.groupByInvite(ctx, r)

	if slices.ContainsFunc(members, func(m *domain.User) bool { return m.ID == me.ID }) {
		http.Redirect(w, r, groupPath(group.ID), http.StatusSeeOther)
		return
	}

	server.RenderPage(w, r, h.SM, "Join "+group.Name, html.JoinMain(group, len(members), group.InviteCode))
}

// Join adds the user to the group.
func (h *GroupHandler) Join(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)
	group, _ := h.groupByInvite(ctx, r)

	if err := model.AddGroupMember(ctx, h.DB, group.ID, me.ID); err != nil {
		panic(err)
	}

	server.Flash(r, h.SM, fmt.Sprintf("Welcome to %s", group.Name))
	http.Redirect(w, r, groupPath(group.ID), http.StatusSeeOther)
}

func (h *GroupHandler) groupByInvite(ctx context.Context, r *http.Request) (*domain.Group, []*domain.User) {
	req := decode[contract.TokenPath](r)

	group, err := model.GetGroupByInviteCode(ctx, h.DB, req.Token)
	if err != nil {
		if errors.Is(err, splitgo.NotFound) {
			panic(splitgo.NotFound.New("Invite link is invalid"))
		}
		panic(err)
	}

	members, err := model.ListGroupMembers(ctx, h.DB, group.ID)
	if err != nil {
		panic(err)
	}

	return group, members
}

func groupForm(g *domain.Group) contract.GroupForm {
	return contract.GroupForm{Name: g.Name, Currency: g.Currency, SimplifyDebts: g.SimplifyDebts}
}

func groupPath(id snowflake.ID) string {
	return "/groups/" + id.String()
}

func today(now time.Time) string {
	return now.Format(domain.DateLayout)
}
