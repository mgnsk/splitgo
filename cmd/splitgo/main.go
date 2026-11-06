// Command splitgo runs the splitgo web application.
package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/alexedwards/scs/bunstore"
	"github.com/mgnsk/splitgo"
	"github.com/mgnsk/splitgo/handler"
	"github.com/mgnsk/splitgo/live"
	"github.com/mgnsk/splitgo/mail"
	"github.com/mgnsk/splitgo/model"
	"github.com/mgnsk/splitgo/push"
	"github.com/mgnsk/splitgo/server"
	"github.com/mgnsk/webkit"
	"github.com/mgnsk/webkit/app"
	"github.com/mgnsk/webkit/sqlite"
)

func main() {
	log.SetFlags(0) // no time prefix

	if err := run(); err != nil {
		slog.Error("error running application", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := LoadConfig()
	if err != nil {
		return webkit.Internal.New("error loading configuration", err)
	}

	databaseDir, err := filepath.Abs(cfg.DatabaseDir)
	if err != nil {
		return webkit.Internal.New("invalid database dir", err)
	}

	if err := os.MkdirAll(databaseDir, 0o755); err != nil {
		return webkit.Internal.New("error creating database dir", err)
	}

	db := sqlite.NewDB(filepath.Join(databaseDir, "splitgo.sqlite")).WithForeignKeys().Connect()
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("error closing database connection", slog.String("error", err.Error()))
		}
	}()

	if err := sqlite.MigrateUp(db.DB, splitgo.Migrations, "migrations"); err != nil {
		return webkit.Internal.New("error migrating database", err)
	}

	if err := sqlite.Optimize(context.Background(), db.DB); err != nil {
		return webkit.Internal.New("error optimizing database", err)
	}

	var mailer mail.Sender = mail.LogSender{}
	if cfg.SMTPURL != "" {
		smtpSender, err := mail.NewSMTPSender(cfg.SMTPURL, cfg.MailFrom)
		if err != nil {
			return webkit.Internal.New("error configuring SMTP", err)
		}
		mailer = smtpSender
	} else {
		slog.Warn("SMTP_URL not set: sign-in links are logged instead of emailed")
	}

	notifier, err := push.New(context.Background(), db, cfg.PushSubscriber())
	if err != nil {
		return webkit.Internal.New("error configuring push notifications", err)
	}
	defer notifier.Wait()

	store, err := bunstore.New(db)
	if err != nil {
		return webkit.Internal.New("error creating sqlite session store", err)
	}

	broker := live.NewBroker()

	deps := &handler.Deps{
		DB:     db,
		SM:     server.NewSessionManager(store, cfg.SecureCookies()),
		Mailer: mailer,
		Push:   notifier,
		Live:   broker,
		Config: handler.Config{
			BaseURL:     cfg.BaseURL,
			AllowSignup: cfg.AllowSignup,
		},
	}

	s := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      handler.New(deps),
		ReadTimeout:  time.Minute,
		WriteTimeout: time.Minute,
		ErrorLog:     slog.NewLogLogger(slog.Default().Handler(), slog.LevelDebug),
	}

	// End open event streams so that shutdown does not wait for them.
	s.RegisterOnShutdown(broker.Close)

	return app.Run(context.Background(), s,
		// Run SQL optimizer periodic task.
		app.Every(24*time.Hour, func(ctx context.Context, _ time.Time) error {
			return sqlite.Optimize(ctx, db.DB)
		}),
		// Run expired login tokens and old invites cleanup periodic task.
		app.Every(5*time.Minute, func(ctx context.Context, now time.Time) error {
			if err := model.DeleteExpiredLoginTokens(ctx, db, now); err != nil {
				return err
			}
			return model.DeleteOldInvites(ctx, db, now)
		}),
	)
}
