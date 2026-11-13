package commands

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pihpohpah/hermes/internal/application/usecase"
)

// UserCreateCommand adds a user from the CLI: local convenience for seeding
// demo profiles without driving the HTTP API. The created user has a session
// too (the use case always issues one), it is simply not printed.
type UserCreateCommand struct {
	logger   *slog.Logger
	useCase  *usecase.CreateUserUseCase
	name     string
	timezone string
}

func NewUserCreateCommand(
	logger *slog.Logger,
	useCase *usecase.CreateUserUseCase,
	name, timezone string,
) *UserCreateCommand {
	return &UserCreateCommand{
		logger:   logger,
		useCase:  useCase,
		name:     name,
		timezone: timezone,
	}
}

func (c *UserCreateCommand) Run(ctx context.Context) error {
	result, err := c.useCase.Do(ctx, usecase.CreateUserInput{
		DisplayName: c.name,
		Timezone:    c.timezone,
	})
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	c.logger.Info(
		"user created",
		slog.String("id", result.User.ID),
		slog.String("display_name", result.User.DisplayName),
		slog.String("timezone", result.User.Timezone),
	)
	fmt.Printf("user id: %s\n", result.User.ID)

	return nil
}
