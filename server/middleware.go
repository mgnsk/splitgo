package server

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"runtime"
	"slices"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/ggicci/httpin/core"
	"github.com/mgnsk/splitgo"
	"github.com/mgnsk/splitgo/html"
	"github.com/mgnsk/splitgo/model"
	"github.com/mgnsk/splitgo/pkg/snowflake"
	"github.com/mgnsk/wreck"
	sloghttp "github.com/samber/slog-http"
	"github.com/uptrace/bun"
	"maragu.dev/gomponents"
	hxhttp "maragu.dev/gomponents-htmx/http"
)

// MiddlewareFunc is a HTTP middleware function.
type MiddlewareFunc func(next http.Handler) http.Handler

// WithMiddleware applies middleware to run in order before handler.
func WithMiddleware(h http.Handler, middlewares ...MiddlewareFunc) http.Handler {
	for _, mw := range slices.Backward(middlewares) {
		h = mw(h)
	}
	return h
}

// NewTimeoutMiddleware creates a new context timeout middleware.
func NewTimeoutMiddleware(timeout time.Duration) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ErrorHandler is a HTTP error handler middleware.
// It renders HTML error pages and logs errors.
func ErrorHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			var err error

			if r := recover(); r == nil {
				return
			} else if r == http.ErrAbortHandler {
				panic(r)
			} else if rerr, ok := r.(error); ok {
				err = rerr
			} else {
				err = fmt.Errorf("%v", r)
			}

			// Default status code and message.
			var (
				code = http.StatusInternalServerError
				msg  = "Something went wrong"
			)

			// Attempt to detect status code and message from error.
			if werr, ok := errors.AsType[*wreck.Error](err); ok {
				// log/slog normalizes int-kind attribute values to int64, so
				// wreck.Value must be read back as int64, not int.
				if v, ok := wreck.Value[int64](werr, splitgo.KeyHTTPCode); ok {
					code = int(v)
				}
				msg = cmp.Or(werr.Message(), msg)
			} else if _, ok := errors.AsType[*core.InvalidFieldError](err); ok {
				// httpin.DecodeTo failed to bind a request field.
				code = http.StatusBadRequest
				msg = "Invalid request"
			} else if errors.Is(err, context.DeadlineExceeded) {
				code = http.StatusGatewayTimeout
				msg = "Timeout"
			}

			if code >= http.StatusInternalServerError {
				stack := make([]byte, 4<<10) // 4 KB
				length := runtime.Stack(stack, false)
				sloghttp.AddCustomAttributes(r, slog.String("stack", string(stack[:length])))
			}
			sloghttp.AddCustomAttributes(r, slog.String("err", err.Error()))

			w.Header().Set("Content-Type", "text/html; charset=UTF-8")

			var node gomponents.Node
			if hxhttp.IsRequest(r.Header) {
				// Let htmx swap the error message into the toast area.
				w.Header().Set("HX-Retarget", "#toast")
				w.Header().Set("HX-Reswap", "innerHTML")
				node = html.Toast(false, msg)
			} else {
				node = html.Page(html.PageProps{
					Title:    "Error",
					Path:     r.URL.Path,
					Children: html.ErrorMain(code, msg),
				})
			}

			w.WriteHeader(code)

			if err := node.Render(w); err != nil {
				slog.Error("error rendering error page", slog.String("error", err.Error()))
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// NewSessionMiddleware creates a new session middleware.
func NewSessionMiddleware(sm *scs.SessionManager) MiddlewareFunc {
	return sm.LoadAndSave
}

// NewUserMiddleware creates a middleware that loads the logged in user
// into the request context.
func NewUserMiddleware(db *bun.DB, sm *scs.SessionManager) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if id := sm.GetInt64(r.Context(), SessionKeyUserID); id != 0 {
				user, err := model.GetUser(r.Context(), db, snowflake.ID(id))
				if err != nil {
					if !errors.Is(err, splitgo.NotFound) {
						panic(err)
					}

					// User has been deleted.
					if err := sm.Destroy(r.Context()); err != nil {
						panic(err)
					}
					http.Redirect(w, r, "/login", http.StatusSeeOther)
					return
				}

				r = r.WithContext(WithUser(r.Context(), user))
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireUser is a middleware that requires a logged in user.
// Anonymous page requests are redirected to the login page.
func RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if GetUser(r.Context()) == nil {
			if r.Method == http.MethodGet && !hxhttp.IsRequest(r.Header) {
				http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
				return
			}
			panic(splitgo.Forbidden.New("Must be logged in"))
		}

		next.ServeHTTP(w, r)
	})
}

// NewCrossOriginMiddleware creates a CSRF protection middleware rejecting
// non-safe cross-origin browser requests.
func NewCrossOriginMiddleware() MiddlewareFunc {
	cop := http.NewCrossOriginProtection()
	cop.SetDenyHandler(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic(splitgo.Forbidden.New("Cross-origin request denied"))
	}))
	return cop.Handler
}
