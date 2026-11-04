// Command splitgo runs the splitgo web application.
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/alexedwards/scs/bunstore"
	"github.com/mgnsk/splitgo"
	"github.com/mgnsk/splitgo/handler"
	"github.com/mgnsk/splitgo/live"
	"github.com/mgnsk/splitgo/mail"
	"github.com/mgnsk/splitgo/model"
	"github.com/mgnsk/splitgo/pkg/sqlite"
	"github.com/mgnsk/splitgo/push"
	"github.com/mgnsk/splitgo/server"
	"golang.org/x/sync/errgroup"
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
		return splitgo.Internal.New("error loading configuration", err)
	}

	databaseDir, err := filepath.Abs(cfg.DatabaseDir)
	if err != nil {
		return splitgo.Internal.New("invalid database dir", err)
	}

	if err := os.MkdirAll(databaseDir, 0o755); err != nil {
		return splitgo.Internal.New("error creating database dir", err)
	}

	db := sqlite.NewDB(filepath.Join(databaseDir, "splitgo.sqlite")).Connect()
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("error closing database connection", slog.String("error", err.Error()))
		}
	}()

	if err := splitgo.MigrateUp(db.DB); err != nil {
		return splitgo.Internal.New("error migrating database", err)
	}

	var mailer mail.Sender = mail.LogSender{}
	if cfg.SMTPURL != "" {
		smtpSender, err := mail.NewSMTPSender(cfg.SMTPURL, cfg.MailFrom)
		if err != nil {
			return splitgo.Internal.New("error configuring SMTP", err)
		}
		mailer = smtpSender
	} else {
		slog.Warn("SMTP_URL not set: sign-in links are logged instead of emailed")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)

	// Run SQL optimizer periodic task.
	g.Go(func() error {
		if err := sqlite.RunOptimizer(ctx, db.DB); err != nil {
			return splitgo.Internal.New("error running sqlite optimizer", err)
		}
		return nil
	})

	// Run expired login tokens and old invites cleanup periodic task.
	g.Go(func() error {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return nil

			case now := <-ticker.C:
				if err := model.DeleteExpiredLoginTokens(ctx, db, now); err != nil {
					return err
				}
				if err := model.DeleteOldInvites(ctx, db, now); err != nil {
					return err
				}
			}
		}
	})

	notifier, err := push.New(ctx, db, cfg.PushSubscriber())
	if err != nil {
		return splitgo.Internal.New("error configuring push notifications", err)
	}
	defer notifier.Wait()

	store, err := bunstore.New(db)
	if err != nil {
		return splitgo.Internal.New("error creating sqlite session store", err)
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

	s := http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      handler.New(deps),
		ReadTimeout:  time.Minute,
		WriteTimeout: time.Minute,
		ErrorLog:     slog.NewLogLogger(slog.Default().Handler(), slog.LevelDebug),
	}

	// End open event streams so that shutdown does not wait for them.
	s.RegisterOnShutdown(broker.Close)

	g.Go(func() error {
		slog.Info(fmt.Sprintf("listening at %s", cfg.ListenAddr))

		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return splitgo.Internal.New("error running server", err)
		}

		return nil
	})

	g.Go(func() error {
		<-ctx.Done()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		slog.Info("shutting down the server")

		if err := s.Shutdown(ctx); err != nil {
			return splitgo.Internal.New("error shutting down server", err)
		}

		return nil
	})

	return g.Wait()
}
