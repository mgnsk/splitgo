package handler

import (
	"fmt"
	"net/http"

	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/html"
	"github.com/mgnsk/webkit/push"
	"github.com/mgnsk/webkit/snowflake"
	"github.com/samber/lo"
)

// ledgerTitle returns the notification title and page for a change in a
// group, seen by the other members. For non-group expenses with a friend
// that is the friend who made the change.
func ledgerTitle(data html.GroupData) (title, path string) {
	if data.Group.Direct() {
		return data.Me.DisplayName(), html.FriendURL(data.Me.ID)
	}
	return data.Group.Name, groupPath(data.Group.ID)
}

// notifyExpense notifies the payer and participants of a new expense.
func (d *Deps) notifyExpense(r *http.Request, data html.GroupData, e *domain.Expense) {
	me := data.Me
	title, _ := ledgerTitle(data)
	path := groupPath(data.Group.ID) + "/expenses/" + e.ID.String()
	total := html.Fmt(e.Amount, e.Currency)

	ids := append([]snowflake.ID{e.PaidBy}, e.ParticipantIDs()...)
	for _, id := range lo.Uniq(lo.Without(ids, me.ID)) {
		body := fmt.Sprintf("%s added %q: %s", me.DisplayName(), e.Description, total)
		if share := e.ShareOf(id); share > 0 {
			body += fmt.Sprintf(", your share %s", html.Fmt(share, e.Currency))
		}
		d.Notifier.Notify(r.Context(), []snowflake.ID{id}, push.Message{
			Title: title,
			Body:  body,
			URL:   path,
			Tag:   e.ID.String(),
		})
	}
}

// notifyExpenseChanged notifies the people involved before or after an
// edit. before are the people involved before the edit.
func (d *Deps) notifyExpenseChanged(r *http.Request, data html.GroupData, e *domain.Expense, before []snowflake.ID) {
	me := data.Me
	title, _ := ledgerTitle(data)
	path := groupPath(data.Group.ID) + "/expenses/" + e.ID.String()
	total := html.Fmt(e.Amount, e.Currency)

	ids := append(append([]snowflake.ID{e.PaidBy}, e.ParticipantIDs()...), before...)
	for _, id := range lo.Uniq(lo.Without(ids, me.ID)) {
		body := fmt.Sprintf("%s edited %q: %s", me.DisplayName(), e.Description, total)
		if share := e.ShareOf(id); share > 0 {
			body += fmt.Sprintf(", your share %s", html.Fmt(share, e.Currency))
		} else if !e.Involves(id) {
			body += ", you're no longer part of it"
		}
		d.Notifier.Notify(r.Context(), []snowflake.ID{id}, push.Message{
			Title: title,
			Body:  body,
			URL:   path,
			Tag:   e.ID.String(),
		})
	}
}

// notifyExpenseDeleted notifies the people involved in a deleted expense or
// payment.
func (d *Deps) notifyExpenseDeleted(r *http.Request, data html.GroupData, e *domain.Expense) {
	me := data.Me
	title, path := ledgerTitle(data)
	total := html.Fmt(e.Amount, e.Currency)

	body := fmt.Sprintf("%s deleted %q (%s)", me.DisplayName(), e.Description, total)
	if e.Kind == domain.KindSettlement {
		body = fmt.Sprintf("%s deleted a payment of %s", me.DisplayName(), total)
	}

	ids := append([]snowflake.ID{e.PaidBy}, e.ParticipantIDs()...)
	d.Notifier.Notify(r.Context(), lo.Uniq(lo.Without(ids, me.ID)), push.Message{
		Title: title,
		Body:  body,
		URL:   path,
		Tag:   e.ID.String(),
	})
}

// notifySettlement notifies the payer and recipient of a recorded payment.
func (d *Deps) notifySettlement(r *http.Request, data html.GroupData, e *domain.Expense, to snowflake.ID) {
	me := data.Me
	amount := html.Fmt(e.Amount, e.Currency)
	title, path := ledgerTitle(data)

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

		d.Notifier.Notify(r.Context(), []snowflake.ID{id}, push.Message{
			Title: title,
			Body:  body,
			URL:   path,
			Tag:   e.ID.String(),
		})
	}
}
