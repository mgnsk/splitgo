// Package handler implements the HTTP handlers.
package handler

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/ggicci/httpin"
	"github.com/mgnsk/splitgo"
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/html"
	"github.com/mgnsk/splitgo/live"
	"github.com/mgnsk/splitgo/mail"
	"github.com/mgnsk/splitgo/model"
	"github.com/mgnsk/splitgo/pkg/snowflake"
	"github.com/mgnsk/splitgo/push"
	"github.com/mgnsk/splitgo/server"
	"github.com/uptrace/bun"
)

// Config is the handler configuration.
type Config struct {
	// BaseURL is the public URL of the app used in emailed links,
	// e.g. https://split.example.com (no trailing slash).
	BaseURL string
	// AllowSignup allows anyone to create an account by signing in.
	// When false, only existing (e.g. invited) users can sign in, except
	// for the very first user.
	AllowSignup bool
	// Now returns the current time. Defaults to time.Now.
	Now func() time.Time
}

// Deps are the dependencies of handlers.
type Deps struct {
	DB     *bun.DB
	SM     *scs.SessionManager
	Mailer mail.Sender
	Push   push.Notifier
	Live   *live.Broker
	Config Config
}

func (d *Deps) now() time.Time {
	if d.Config.Now != nil {
		return d.Config.Now()
	}
	return time.Now()
}

// middlewares returns the common middlewares for pages.
func (d *Deps) middlewares(requireUser bool) []server.MiddlewareFunc {
	mws := []server.MiddlewareFunc{
		server.NewSessionMiddleware(d.SM),
		server.NewUserMiddleware(d.DB, d.SM),
	}
	if requireUser {
		mws = append(mws, server.RequireUser)
	}
	return mws
}

func (d *Deps) handle(mux *http.ServeMux, pattern string, h http.HandlerFunc, requireUser bool) {
	mux.Handle(pattern, server.WithMiddleware(h, d.middlewares(requireUser)...))
}

// mustUser returns the logged in user.
func mustUser(r *http.Request) *domain.User {
	user := server.GetUser(r.Context())
	if user == nil {
		panic(splitgo.Forbidden.New("Must be logged in"))
	}
	return user
}

// decode decodes the request into a new V or panics.
func decode[V any](r *http.Request) *V {
	v := new(V)
	if err := httpin.DecodeTo(r, v); err != nil {
		panic(err)
	}
	return v
}

// loadGroup loads a group the user is a member of along with its members.
// Non-members get NotFound.
func (d *Deps) loadGroup(ctx context.Context, me *domain.User, groupID snowflake.ID) html.GroupData {
	group, err := model.GetUserGroup(ctx, d.DB, me.ID, groupID)
	if err != nil {
		if errors.Is(err, splitgo.NotFound) {
			panic(splitgo.NotFound.New("Group not found"))
		}
		panic(err)
	}

	members, err := model.ListGroupMembers(ctx, d.DB, groupID)
	if err != nil {
		panic(err)
	}

	people := html.People{}
	for _, m := range members {
		people[m.ID] = m
	}

	return html.GroupData{Me: me, Group: group, Members: members, People: people}
}

// loadOrdinaryGroup loads a group like loadGroup, rejecting the hidden
// groups holding non-group expenses with a friend: they have no settings
// or membership of their own.
func (d *Deps) loadOrdinaryGroup(ctx context.Context, me *domain.User, groupID snowflake.ID) html.GroupData {
	data := d.loadGroup(ctx, me, groupID)
	if data.Group.Direct() {
		panic(splitgo.NotFound.New("Group not found"))
	}
	return data
}

// addPeople loads users referenced by ids that are not yet in people.
func (d *Deps) addPeople(ctx context.Context, people html.People, ids []snowflake.ID) {
	var missing []snowflake.ID
	for _, id := range ids {
		if _, ok := people[id]; !ok {
			missing = append(missing, id)
		}
	}

	users, err := model.ListUsersByIDs(ctx, d.DB, missing)
	if err != nil {
		panic(err)
	}

	for id, u := range users {
		people[id] = u
	}
}

func expensePeopleIDs(expenses ...*domain.Expense) []snowflake.ID {
	var ids []snowflake.ID
	for _, e := range expenses {
		ids = append(ids, e.PaidBy, e.AddedBy)
		for _, p := range e.Participants {
			ids = append(ids, p.UserID)
		}
	}
	return ids
}

func sortedUsers(people html.People) []*domain.User {
	users := make([]*domain.User, 0, len(people))
	for _, u := range people {
		users = append(users, u)
	}
	slices.SortFunc(users, func(a, b *domain.User) int { return cmp.Compare(a.ID, b.ID) })
	return users
}

func memberIDs(members []*domain.User) []snowflake.ID {
	ids := make([]snowflake.ID, len(members))
	for i, m := range members {
		ids[i] = m.ID
	}
	return ids
}

// randomToken returns a random URL-safe token.
func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// hashToken returns the hex SHA-256 of a token.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// safeNext returns next if it is a local path, otherwise "/".
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/"
	}
	return next
}

// New builds the application HTTP handler with all routes.
func New(deps *Deps) http.Handler {
	return server.NewHandler(
		splitgo.RegisterAssetsHandler,
		NewAuthHandler(deps).Register,
		NewHomeHandler(deps).Register,
		NewGroupHandler(deps).Register,
		NewExpenseHandler(deps).Register,
		NewSettleHandler(deps).Register,
		NewPushHandler(deps).Register,
		NewEventsHandler(deps).Register,
		NewFriendHandler(deps).Register,
	)
}
