package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/mgnsk/splitgo/contract"
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/html"
	"github.com/mgnsk/splitgo/model"
	"github.com/mgnsk/splitgo/server"
	"github.com/mgnsk/webkit"
	"github.com/mgnsk/webkit/session"
	"github.com/mgnsk/webkit/snowflake"
	"github.com/mgnsk/webkit/web"
)

// ExpenseHandler handles expenses.
type ExpenseHandler struct {
	*Deps
}

// NewExpenseHandler creates a new expense handler.
func NewExpenseHandler(deps *Deps) *ExpenseHandler {
	return &ExpenseHandler{Deps: deps}
}

// Register the handler.
func (h *ExpenseHandler) Register(mux *http.ServeMux) {
	h.handle(mux, "GET /groups/{group_id}/expenses/new", h.New, true)
	h.handle(mux, "POST /groups/{group_id}/expenses", h.Create, true)
	h.handle(mux, "POST /groups/{group_id}/expenses/split", h.Split, true)
	h.handle(mux, "GET /groups/{group_id}/expenses/{expense_id}", h.Show, true)
	h.handle(mux, "GET /groups/{group_id}/expenses/{expense_id}/edit", h.Edit, true)
	h.handle(mux, "POST /groups/{group_id}/expenses/{expense_id}", h.Update, true)
	h.handle(mux, "POST /groups/{group_id}/expenses/{expense_id}/delete", h.Delete, true)
}

// New renders the add expense form.
func (h *ExpenseHandler) New(w http.ResponseWriter, r *http.Request) {
	me := mustUser(r)
	req := web.Decode[contract.GroupPath](r)
	d := h.loadGroup(r.Context(), me, req.GroupID)

	form := contract.NewExpenseForm(d.Group, me.ID, h.userNow(me))

	h.renderForm(w, r, html.ExpenseFormData{
		GroupData:    d,
		Participants: d.Members,
		Form:         form,
	})
}

// Create creates an expense.
func (h *ExpenseHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)
	req := web.Decode[contract.GroupPath](r)
	d := h.loadGroup(ctx, me, req.GroupID)

	form := parseForm(r)
	resolved, errs := form.Validate(me.ID, d.Members)
	if len(errs) > 0 {
		h.renderForm(w, r, html.ExpenseFormData{GroupData: d, Participants: d.Members, Form: form, Errors: errs})
		return
	}

	e := &domain.Expense{
		ID:      snowflake.Generate(),
		GroupID: d.Group.ID,
		Kind:    domain.KindExpense,
		AddedBy: me.ID,
	}
	applyResolved(e, resolved, h.now())

	if err := model.InsertExpense(ctx, h.DB, e); err != nil {
		panic(err)
	}

	h.notifyExpense(r, d, e)

	h.publishGroup(d.Group.ID, d.Members)

	session.Flash(r.Context(), h.SM, "Expense added")
	http.Redirect(w, r, d.LedgerURL(), http.StatusSeeOther)
}

// Split renders the split section, its values or its live preview (htmx).
func (h *ExpenseHandler) Split(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)
	req := web.Decode[contract.GroupPath](r)
	d := h.loadGroup(ctx, me, req.GroupID)

	participants := d.Members
	if id, ok := parseFormID(r.PostFormValue("expense_id")); ok {
		participants = h.expenseParticipants(ctx, d, h.getExpense(ctx, d, id))
	}

	form := parseForm(r)
	switch r.PostFormValue("switch") {
	case contract.ModeAdvanced:
		form.SwitchToAdvanced(me.ID, participants)
	case contract.ModeSimple:
		form.Mode = contract.ModeSimple
	}

	data := html.ExpenseFormData{GroupData: d, Participants: participants, Form: form}

	switch r.URL.Query().Get("part") {
	case "preview":
		web.Render(w, html.SplitPreview(data))
	case "values":
		web.Render(w, html.SplitValues(data))
	default:
		web.Render(w, html.SplitSection(data))
	}
}

// Show renders an expense.
func (h *ExpenseHandler) Show(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)
	req := web.Decode[contract.ExpensePath](r)
	d := h.loadGroup(ctx, me, req.GroupID)
	e := h.getExpense(ctx, d, req.ExpenseID)

	h.addPeople(ctx, d.People, expensePeopleIDs(e))

	server.RenderPage(w, r, h.SM, e.Description, html.ExpenseDetailMain(d, e))
}

// Edit renders the edit expense form.
func (h *ExpenseHandler) Edit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)
	req := web.Decode[contract.ExpensePath](r)
	d := h.loadGroup(ctx, me, req.GroupID)
	e := h.getEditableExpense(ctx, d, req.ExpenseID)

	participants := h.expenseParticipants(ctx, d, e)

	h.renderForm(w, r, html.ExpenseFormData{
		GroupData:    d,
		Participants: participants,
		Form:         contract.ExpenseFormFromExpense(e, me.ID, participants),
		ExpenseID:    e.ID,
	})
}

// Update updates an expense.
func (h *ExpenseHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)
	req := web.Decode[contract.ExpensePath](r)
	d := h.loadGroup(ctx, me, req.GroupID)
	e := h.getEditableExpense(ctx, d, req.ExpenseID)

	participants := h.expenseParticipants(ctx, d, e)

	form := parseForm(r)
	resolved, errs := form.Validate(me.ID, participants)
	if len(errs) > 0 {
		h.renderForm(w, r, html.ExpenseFormData{
			GroupData:    d,
			Participants: participants,
			Form:         form,
			Errors:       errs,
			ExpenseID:    e.ID,
		})
		return
	}

	before := append([]snowflake.ID{e.PaidBy}, e.ParticipantIDs()...)
	applyResolved(e, resolved, h.now())

	if err := model.UpdateExpense(ctx, h.DB, e); err != nil {
		panic(err)
	}

	h.notifyExpenseChanged(r, d, e, before)

	h.publishGroup(d.Group.ID, d.Members)

	session.Flash(r.Context(), h.SM, "Expense saved")
	http.Redirect(w, r, groupPath(d.Group.ID)+"/expenses/"+e.ID.String(), http.StatusSeeOther)
}

// Delete soft deletes an expense or payment.
func (h *ExpenseHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := mustUser(r)
	req := web.Decode[contract.ExpensePath](r)
	d := h.loadGroup(ctx, me, req.GroupID)
	e := h.getExpense(ctx, d, req.ExpenseID)

	if err := model.DeleteExpense(ctx, h.DB, e.ID, h.now()); err != nil {
		panic(err)
	}

	h.notifyExpenseDeleted(r, d, e)

	msg := "Expense deleted"
	if e.Kind == domain.KindSettlement {
		msg = "Payment deleted"
	}

	h.publishGroup(d.Group.ID, d.Members)

	session.Flash(r.Context(), h.SM, msg)
	web.Redirect(w, r, d.LedgerURL())
}

func (h *ExpenseHandler) renderForm(w http.ResponseWriter, r *http.Request, data html.ExpenseFormData) {
	title := "Add expense"
	if data.ExpenseID != 0 {
		title = "Edit expense"
	}
	server.RenderPage(w, r, h.SM, title, html.ExpenseFormMain(data))
}

func (h *ExpenseHandler) getExpense(ctx context.Context, d html.GroupData, id snowflake.ID) *domain.Expense {
	e, err := model.GetGroupExpense(ctx, h.DB, d.Group.ID, id)
	if err != nil {
		if errors.Is(err, webkit.NotFound) {
			panic(webkit.NotFound.New("Expense not found"))
		}
		panic(err)
	}
	return e
}

func (h *ExpenseHandler) getEditableExpense(ctx context.Context, d html.GroupData, id snowflake.ID) *domain.Expense {
	e := h.getExpense(ctx, d, id)
	if e.Kind != domain.KindExpense {
		panic(webkit.InvalidValue.New("Payments can not be edited, delete and record it again"))
	}
	return e
}

// expenseParticipants returns the current members plus any former members
// referenced by the expense, sorted by ID.
func (h *ExpenseHandler) expenseParticipants(ctx context.Context, d html.GroupData, e *domain.Expense) []*domain.User {
	people := html.People{}
	for _, m := range d.Members {
		people[m.ID] = m
	}

	ids := []snowflake.ID{e.PaidBy}
	for _, p := range e.Participants {
		ids = append(ids, p.UserID)
	}
	h.addPeople(ctx, people, ids)

	return sortedUsers(people)
}

func parseForm(r *http.Request) *contract.ExpenseForm {
	if err := r.ParseForm(); err != nil {
		panic(webkit.InvalidValue.New("Invalid form", err))
	}
	return contract.ParseExpenseForm(r.PostForm)
}

func parseFormID(s string) (snowflake.ID, bool) {
	var id snowflake.ID
	if s == "" || id.UnmarshalText([]byte(s)) != nil || id == 0 {
		return 0, false
	}
	return id, true
}

func applyResolved(e *domain.Expense, r *contract.ResolvedExpense, now time.Time) {
	e.Description = r.Description
	e.Amount = r.Amount
	e.Currency = r.Currency.Code
	e.Date = r.Date
	e.PaidBy = r.PaidBy
	e.SplitType = r.SplitType
	e.Participants = r.Participants
	e.UpdatedAt = now
}
