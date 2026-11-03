package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/mgnsk/splitgo"
	"github.com/mgnsk/splitgo/contract"
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/html"
	"github.com/mgnsk/splitgo/mail"
	"github.com/mgnsk/splitgo/model"
	"github.com/mgnsk/splitgo/money"
	"github.com/mgnsk/splitgo/pkg/snowflake"
	"github.com/mgnsk/splitgo/server"
)

const (
	// LoginTokenTTL is the lifetime of sign-in links.
	LoginTokenTTL = 15 * time.Minute
	// InviteTokenTTL is the lifetime of sign-in links sent with invitations.
	InviteTokenTTL = 7 * 24 * time.Hour
	// loginRateLimit is the minimum interval between sign-in emails per address.
	loginRateLimit = time.Minute
)

// AuthHandler handles magic link authentication.
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

// Login emails a sign-in link.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	req := decode[contract.LoginForm](r)
	next := safeNext(req.Next)

	if errs := req.Validate(); len(errs) > 0 {
		server.RenderPage(w, r, h.SM, "Sign in", html.LoginMain(req.Email, next, errs))
		return
	}

	email := domain.NormalizeEmail(req.Email)

	allowed, err := h.canSignIn(r.Context(), email)
	if err != nil {
		panic(err)
	}

	if allowed {
		if err := h.sendLoginLink(r.Context(), email, next); err != nil {
			panic(err)
		}
	} else {
		slog.Info("sign-in requested for unknown email with signup disabled")
	}

	// Always show the same page to not reveal which emails have accounts.
	server.RenderPage(w, r, h.SM, "Check your email", html.CheckEmailMain(email))
}

func (h *AuthHandler) sendLoginLink(ctx context.Context, email, next string) error {
	last, err := model.LastLoginTokenTime(ctx, h.DB, email)
	if err != nil {
		return err
	}

	if !last.IsZero() && h.now().Sub(last) < loginRateLimit {
		slog.Info("sign-in email rate limited")
		return nil
	}

	link, err := h.createLoginLink(ctx, email, next, LoginTokenTTL)
	if err != nil {
		return err
	}

	return h.Mailer.Send(ctx, mail.Message{
		To:      email,
		Subject: "Your splitgo sign-in link",
		Body: fmt.Sprintf("Hi,\n\nUse this link to sign in to splitgo:\n\n%s\n\n"+
			"The link expires in 15 minutes and can be used once.\n"+
			"If you didn't request it, you can ignore this email.\n", link),
	})
}

// createLoginLink stores a new login token and returns the sign-in URL.
func (d *Deps) createLoginLink(ctx context.Context, email, next string, ttl time.Duration) (string, error) {
	token := randomToken(32)

	if err := model.InsertLoginToken(ctx, d.DB, hashToken(token), email, d.now(), ttl); err != nil {
		return "", err
	}

	link := fmt.Sprintf("%s/login/%s", d.Config.BaseURL, token)
	if next != "/" && next != "" {
		link += "?next=" + url.QueryEscape(next)
	}

	return link, nil
}

// canSignIn reports whether email may sign in.
func (h *AuthHandler) canSignIn(ctx context.Context, email string) (bool, error) {
	if _, err := model.GetUserByEmail(ctx, h.DB, email); err == nil {
		return true, nil
	} else if !errors.Is(err, splitgo.NotFound) {
		return false, err
	}

	if h.Config.AllowSignup {
		return true, nil
	}

	// The first user can always sign up.
	n, err := model.CountUsers(ctx, h.DB)
	if err != nil {
		return false, err
	}

	return n == 0, nil
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
	ctx := r.Context()

	email, err := model.ConsumeLoginToken(ctx, h.DB, hashToken(req.Token), h.now())
	if err != nil {
		if errors.Is(err, splitgo.NotFound) {
			server.RenderPage(w, r, h.SM, "Link expired", html.InvalidLinkMain())
			return
		}
		panic(err)
	}

	user, err := model.GetUserByEmail(ctx, h.DB, email)
	if err != nil {
		if !errors.Is(err, splitgo.NotFound) {
			panic(err)
		}

		allowed, err := h.canSignIn(ctx, email)
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

	// First renew the session token.
	if err := h.SM.RenewToken(ctx); err != nil {
		panic(err)
	}

	// Then make the privilege-level change.
	h.SM.Put(ctx, server.SessionKeyUserID, user.ID.Int64())

	http.Redirect(w, r, safeNext(r.URL.Query().Get("next")), http.StatusSeeOther)
}

// Logout logs out.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if err := h.SM.Destroy(r.Context()); err != nil {
		panic(err)
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
