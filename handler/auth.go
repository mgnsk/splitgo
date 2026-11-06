package handler

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mgnsk/splitgo"
	"github.com/mgnsk/splitgo/contract"
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/html"
	"github.com/mgnsk/splitgo/mail"
	"github.com/mgnsk/splitgo/model"
	"github.com/mgnsk/splitgo/money"
	"github.com/mgnsk/splitgo/server"
	"github.com/mgnsk/webkit/snowflake"
)

const (
	// LoginTokenTTL is the lifetime of sign-in links and codes.
	LoginTokenTTL = 15 * time.Minute
	// InviteTokenTTL is the lifetime of sign-in links sent with invitations.
	InviteTokenTTL = 7 * 24 * time.Hour
	// loginRateLimit is the minimum interval between sign-in emails per address.
	loginRateLimit = time.Minute
	// maxLoginFailures is the number of wrong codes per address within
	// model.LoginFailureWindow after which codes are refused. Links still work.
	maxLoginFailures = 20
)

// AuthHandler handles passwordless authentication with emailed links and
// one-time codes. Codes allow signing in from an installed app (PWA): on iOS
// it gets a copy of Safari's cookies only when added to the Home Screen, so a
// link opened in the browser later does not sign the app in.
type AuthHandler struct {
	*Deps
}

// NewAuthHandler creates a new authentication handler.
func NewAuthHandler(deps *Deps) *AuthHandler {
	return &AuthHandler{Deps: deps}
}

// Register the handler.
func (h *AuthHandler) Register(mux *http.ServeMux) {
	h.handle(mux, "GET /login", h.LoginPage, false)
	h.handle(mux, "POST /login", h.Login, false)
	h.handle(mux, "POST /login/code", h.Code, false)
	h.handle(mux, "GET /login/{token}", h.ConfirmPage, false)
	h.handle(mux, "POST /login/{token}", h.Confirm, false)
	h.handle(mux, "POST /logout", h.Logout, false)
}

// LoginPage renders the login form.
func (h *AuthHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	if server.GetUser(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	server.RenderPage(w, r, h.SM, "Sign in", html.LoginMain("", safeNext(r.URL.Query().Get("next")), nil))
}

// Login emails a sign-in link and code.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	req := decode[contract.LoginForm](r)
	next := safeNext(req.Next)

	if errs := req.Validate(); len(errs) > 0 {
		server.RenderPage(w, r, h.SM, "Sign in", html.LoginMain(req.Email, next, errs))
		return
	}

	email := domain.NormalizeEmail(req.Email)

	allowed, err := h.canSignIn(r.Context(), email, next)
	if err != nil {
		panic(err)
	}

	if allowed {
		if err := h.sendLoginEmail(r.Context(), email, next); err != nil {
			panic(err)
		}
	} else {
		slog.Info("sign-in requested for unknown email with signup disabled")
	}

	// Always show the same page to not reveal which emails have accounts.
	server.RenderPage(w, r, h.SM, "Check your email", html.CheckEmailMain(email, next, nil))
}

func (h *AuthHandler) sendLoginEmail(ctx context.Context, email, next string) error {
	last, err := model.LastLoginTokenTime(ctx, h.DB, email)
	if err != nil {
		return err
	}

	if !last.IsZero() && h.now().Sub(last) < loginRateLimit {
		slog.Info("sign-in email rate limited")
		return nil
	}

	link, code, err := h.createLogin(ctx, email, next, LoginTokenTTL, true)
	if err != nil {
		return err
	}

	return h.Mailer.Send(ctx, mail.Message{
		To:      email,
		Subject: fmt.Sprintf("Your splitgo sign-in code: %s", code),
		Body: fmt.Sprintf("Hi,\n\nYour splitgo sign-in code is:\n\n    %s\n\n"+
			"Or use this link to sign in:\n\n%s\n\n"+
			"The code and link expire in 15 minutes and can be used once.\n"+
			"If you didn't request them, you can ignore this email.\n", code, link),
	})
}

// createLogin stores a new login token and returns the sign-in URL and, if
// withCode is set, a 6-digit one-time code.
func (d *Deps) createLogin(ctx context.Context, email, next string, ttl time.Duration, withCode bool) (link, code string, err error) {
	token := randomToken(32)
	tokenHash := hashToken(token)

	var codeHash string
	if withCode {
		code = randomCode()
		codeHash = hashCode(tokenHash, code)
	}

	if err := model.InsertLoginToken(ctx, d.DB, tokenHash, codeHash, email, d.now(), ttl); err != nil {
		return "", "", err
	}

	link = fmt.Sprintf("%s/login/%s", d.Config.BaseURL, token)
	if next != "/" && next != "" {
		link += "?next=" + url.QueryEscape(next)
	}

	return link, code, nil
}

// randomCode returns a random 6-digit code.
func randomCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%06d", n.Int64())
}

// hashCode hashes a code bound to its token.
func hashCode(tokenHash, code string) string {
	return hashToken(tokenHash + ":" + code)
}

// canSignIn reports whether email may sign in. With signup disabled, new
// accounts are still allowed when signing in to follow a valid group invite
// link (next is /join/<code>).
func (h *AuthHandler) canSignIn(ctx context.Context, email, next string) (bool, error) {
	if _, err := model.GetUserByEmail(ctx, h.DB, email); err == nil {
		return true, nil
	} else if !errors.Is(err, splitgo.NotFound) {
		return false, err
	}

	if h.Config.AllowSignup {
		return true, nil
	}

	if code, ok := strings.CutPrefix(next, "/join/"); ok && code != "" && !strings.Contains(code, "/") {
		if _, err := model.GetGroupByInviteCode(ctx, h.DB, code); err == nil {
			return true, nil
		} else if !errors.Is(err, splitgo.NotFound) {
			return false, err
		}
	}

	// The first user can always sign up.
	n, err := model.CountUsers(ctx, h.DB)
	if err != nil {
		return false, err
	}

	return n == 0, nil
}

// Code signs in with an emailed one-time code.
func (h *AuthHandler) Code(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	req := decode[contract.CodeForm](r)
	next := safeNext(req.Next)
	email := domain.NormalizeEmail(req.Email)

	render := func(msg string) {
		server.RenderPage(w, r, h.SM, "Check your email", html.CheckEmailMain(email, next, url.Values{"code": {msg}}))
	}

	if errs := req.Validate(); len(errs) > 0 {
		server.RenderPage(w, r, h.SM, "Check your email", html.CheckEmailMain(email, next, errs))
		return
	}

	failures, err := model.CountLoginFailures(ctx, h.DB, email, h.now().Add(-model.LoginFailureWindow))
	if err != nil {
		panic(err)
	}
	if failures >= maxLoginFailures {
		render("Too many wrong codes. Use the link in the email instead.")
		return
	}

	err = model.ConsumeLoginCode(ctx, h.DB, email, func(tokenHash string) string {
		return hashCode(tokenHash, req.Code)
	}, h.now())
	switch {
	case errors.Is(err, splitgo.NotFound):
		render("This code has expired. Request a new one.")
		return
	case errors.Is(err, splitgo.InvalidValue):
		render("Wrong code. Check the email and try again.")
		return
	case err != nil:
		panic(err)
	}

	h.signIn(w, r, email, next)
}

// ConfirmPage renders a page asking to confirm the sign in. Confirming with
// POST prevents email link scanners from consuming the token.
func (h *AuthHandler) ConfirmPage(w http.ResponseWriter, r *http.Request) {
	req := decode[contract.TokenPath](r)

	email, err := model.GetLoginToken(r.Context(), h.DB, hashToken(req.Token), h.now())
	if err != nil {
		if errors.Is(err, splitgo.NotFound) {
			server.RenderPage(w, r, h.SM, "Link expired", html.InvalidLinkMain())
			return
		}
		panic(err)
	}

	action := "/login/" + req.Token
	if next := safeNext(r.URL.Query().Get("next")); next != "/" {
		action += "?next=" + url.QueryEscape(next)
	}

	server.RenderPage(w, r, h.SM, "Sign in", html.ConfirmLoginMain(action, email))
}

// Confirm consumes the sign-in token and logs the user in.
func (h *AuthHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	req := decode[contract.TokenPath](r)

	email, err := model.ConsumeLoginToken(r.Context(), h.DB, hashToken(req.Token), h.now())
	if err != nil {
		if errors.Is(err, splitgo.NotFound) {
			server.RenderPage(w, r, h.SM, "Link expired", html.InvalidLinkMain())
			return
		}
		panic(err)
	}

	h.signIn(w, r, email, safeNext(r.URL.Query().Get("next")))
}

// signIn logs in the user with a verified email, creating the account if
// allowed, and redirects to next.
func (h *AuthHandler) signIn(w http.ResponseWriter, r *http.Request, email, next string) {
	ctx := r.Context()

	user, err := model.GetUserByEmail(ctx, h.DB, email)
	if err != nil {
		if !errors.Is(err, splitgo.NotFound) {
			panic(err)
		}

		allowed, err := h.canSignIn(ctx, email, next)
		if err != nil {
			panic(err)
		}
		if !allowed {
			server.RenderPage(w, r, h.SM, "Link expired", html.InvalidLinkMain())
			return
		}

		user = &domain.User{ID: snowflake.Generate(), Email: email, Currency: money.DefaultCurrency}
		if err := model.InsertUser(ctx, h.DB, user); err != nil {
			panic(err)
		}
	}

	// The sign-in forms carry the browser's time zone, so dates are local
	// from the first page.
	if tz := r.PostFormValue("timezone"); tz != user.Timezone && domain.ValidTimezone(tz) {
		if err := model.SetUserTimezone(ctx, h.DB, user.ID, tz); err != nil {
			panic(err)
		}
	}

	// First renew the session token.
	if err := h.SM.RenewToken(ctx); err != nil {
		panic(err)
	}

	// Then make the privilege-level change.
	h.SM.Put(ctx, server.SessionKeyUserID, user.ID.Int64())

	http.Redirect(w, r, next, http.StatusSeeOther)
}

// Logout logs out.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if err := h.SM.Destroy(r.Context()); err != nil {
		panic(err)
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
