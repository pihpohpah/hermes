package commands

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/pihpohpah/hermes/internal/application/usecase"
)

// PinsCleanupCommand physically deletes radar pins whose grace period after
// ends_at has passed. Reading filters expired pins out on its own, so this is
// only storage hygiene; run it on a schedule (cron) or by hand.
type PinsCleanupCommand struct {
	logger *slog.Logger
	purger *usecase.PurgeExpiredPinsUseCase
	grace  time.Duration
}

func NewPinsCleanupCommand(
	logger *slog.Logger,
	purger *usecase.PurgeExpiredPinsUseCase,
	grace time.Duration,
) *PinsCleanupCommand {
	return &PinsCleanupCommand{
		logger: logger,
		purger: purger,
		grace:  grace,
	}
}

func (c *PinsCleanupCommand) Run(ctx context.Context) error {
	deleted, err := c.purger.Do(ctx, c.grace)
	if err != nil {
		return fmt.Errorf("purge expired pins: %w", err)
	}

	c.logger.Info("expired pins purged", slog.Int64("deleted", deleted))

	return nil
}
