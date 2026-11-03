package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// RunOptimizer runs an sqlite database periodic optimizer task.
func RunOptimizer(ctx context.Context, db *sql.DB) error {
	start := time.Now()

	// Run thorough optimize on startup.
	if _, err := db.ExecContext(ctx, `PRAGMA optimize=0x10002`); err != nil {
		return err
	}

	slog.Info(fmt.Sprintf("database optimization finished in %v", time.Since(start)))

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-ticker.C:
			if _, err := db.ExecContext(ctx, `PRAGMA optimize`); err != nil {
				return err
			}
		}
	}
}
