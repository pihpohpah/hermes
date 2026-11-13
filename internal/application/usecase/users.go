package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// IdempotencyScopeUserCreate namespaces keys of POST /users so the same
// mechanism can be reused by other mutating endpoints later.
const IdempotencyScopeUserCreate = "users.create"

// CreateUserInput is everything POST /users needs: the profile fields and the
// idempotency contract (empty key means "no idempotency", which is allowed).
type CreateUserInput struct {
	DisplayName string
	Timezone    string

	IdempotencyKey     string
	RequestFingerprint string
}

// CreateUserResult carries the created profile plus the opaque session
// credentials. Replayed is true when the user already existed under the
// idempotency key; a fresh session is issued anyway, so a client that lost
// the first response still ends up with a working cookie while no second
// user is ever created.
type CreateUserResult struct {
	User             models.User
	SessionToken     string
	CSRFToken        string
	SessionExpiresAt time.Time
	Replayed         bool
}

type userCreator interface {
	Create(ctx context.Context, user models.User) (models.User, error)
	GetByID(ctx context.Context, id string) (models.User, error)
}

type sessionCreator interface {
	Create(ctx context.Context, session models.UserSession) (models.UserSession, error)
}

type idempotencyStore interface {
	Get(ctx context.Context, scope, key string) (models.IdempotencyRecord, error)
	// Save inserts the record and reports whether it happened; false means a
	// concurrent request already claimed the (scope, key) pair.
	Save(ctx context.Context, record models.IdempotencyRecord) (bool, error)
}

type txRunner interface {
	Run(ctx context.Context, fn func(ctx context.Context) error) error
}

// CreateUserUseCase creates the guest profile and a server session atomically
// and pins the outcome to the idempotency key when one is supplied.
type CreateUserUseCase struct {
	users    userCreator
	sessions sessionCreator
	idem     idempotencyStore
	tx       txRunner
	ttl      time.Duration
}

func NewCreateUserUseCase(
	users userCreator,
	sessions sessionCreator,
	idem idempotencyStore,
	tx txRunner,
	ttl time.Duration,
) *CreateUserUseCase {
	return &CreateUserUseCase{
		users:    users,
		sessions: sessions,
		idem:     idem,
		tx:       tx,
		ttl:      ttl,
	}
}

func (u *CreateUserUseCase) Do(ctx context.Context, in CreateUserInput) (CreateUserResult, error) {
	user, err := models.NewUser(in.DisplayName, in.Timezone)
	if err != nil {
		return CreateUserResult{}, err
	}

	if in.IdempotencyKey != "" {
		stored, found, err := u.storedUser(ctx, in)
		if err != nil {
			return CreateUserResult{}, err
		}

		if found {
			return u.withFreshSession(ctx, stored, true)
		}
	}

	result, err := u.create(ctx, user, in)
	if err == nil {
		return result, nil
	}

	// A concurrent request with the same key committed between our check and
	// insert; resolve the outcome from the winner's record.
	if errors.Is(err, errRetryIdempotency) {
		stored, found, lookupErr := u.storedUser(ctx, in)
		if lookupErr != nil {
			return CreateUserResult{}, lookupErr
		}

		if found {
			return u.withFreshSession(ctx, stored, true)
		}

		return CreateUserResult{}, fmt.Errorf("%w: idempotency key race lost", models.ErrConflict)
	}

	return CreateUserResult{}, err
}

// storedUser resolves the user pinned to the idempotency key. A fingerprint
// mismatch is a client error (409).
func (u *CreateUserUseCase) storedUser(
	ctx context.Context,
	in CreateUserInput,
) (models.User, bool, error) {
	record, err := u.idem.Get(ctx, IdempotencyScopeUserCreate, in.IdempotencyKey)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return models.User{}, false, nil
		}

		return models.User{}, false, fmt.Errorf("load idempotency record: %w", err)
	}

	if record.RequestFingerprint != in.RequestFingerprint {
		return models.User{}, false, fmt.Errorf(
			"%w: Idempotency-Key `%s` was already used with a different request body",
			models.ErrConflict, in.IdempotencyKey,
		)
	}

	stored, err := u.users.GetByID(ctx, record.UserID)
	if err != nil {
		return models.User{}, false, fmt.Errorf("load user by idempotency record: %w", err)
	}

	return stored, true, nil
}

// withFreshSession issues a new session for an already existing user (replay
// path). The user row is never touched here.
func (u *CreateUserUseCase) withFreshSession(
	ctx context.Context,
	user models.User,
	replayed bool,
) (CreateUserResult, error) {
	result := CreateUserResult{User: user, Replayed: replayed}

	err := u.tx.Run(ctx, func(txCtx context.Context) error {
		sessionToken, csrfToken, expiresAt, err := u.issueSession(txCtx, user.ID, time.Now())
		if err != nil {
			return err
		}

		result.SessionToken = sessionToken
		result.CSRFToken = csrfToken
		result.SessionExpiresAt = expiresAt

		return nil
	})
	if err != nil {
		return CreateUserResult{}, err
	}

	return result, nil
}

func (u *CreateUserUseCase) create(
	ctx context.Context,
	user models.User,
	in CreateUserInput,
) (CreateUserResult, error) {
	var result CreateUserResult

	now := time.Now()

	err := u.tx.Run(ctx, func(txCtx context.Context) error {
		created, err := u.users.Create(txCtx, user)
		if err != nil {
			return fmt.Errorf("create user: %w", err)
		}

		sessionToken, csrfToken, expiresAt, err := u.issueSession(txCtx, created.ID, now)
		if err != nil {
			return err
		}

		if in.IdempotencyKey != "" {
			record := models.IdempotencyRecord{
				Scope:              IdempotencyScopeUserCreate,
				Key:                in.IdempotencyKey,
				RequestFingerprint: in.RequestFingerprint,
				UserID:             created.ID,
				ExpiresAt:          now.Add(u.ttl),
			}

			inserted, err := u.idem.Save(txCtx, record)
			if err != nil {
				return fmt.Errorf("save idempotency record: %w", err)
			}

			if !inserted {
				// A concurrent request won the key; roll back and re-read.
				return errRetryIdempotency
			}
		}

		result = CreateUserResult{
			User:             created,
			SessionToken:     sessionToken,
			CSRFToken:        csrfToken,
			SessionExpiresAt: expiresAt,
		}

		return nil
	})
	if err != nil {
		return CreateUserResult{}, err
	}

	return result, nil
}

// issueSession persists a session for userID and returns the opaque tokens
// together with the expiry. It must be called inside u.tx.Run.
func (u *CreateUserUseCase) issueSession(
	ctx context.Context,
	userID string,
	now time.Time,
) (sessionToken string, csrfToken string, expiresAt time.Time, err error) {
	sessionToken, sessionHash, err := models.NewOpaqueToken()
	if err != nil {
		return "", "", time.Time{}, err
	}

	csrfToken, csrfHash, err := models.NewOpaqueToken()
	if err != nil {
		return "", "", time.Time{}, err
	}

	session, err := models.NewUserSession(userID, sessionHash, csrfHash, now, u.ttl)
	if err != nil {
		return "", "", time.Time{}, err
	}

	stored, err := u.sessions.Create(ctx, session)
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("create session: %w", err)
	}

	return sessionToken, csrfToken, stored.ExpiresAt, nil
}

// errRetryIdempotency signals that another request won the race for the same
// idempotency key and the outcome must be re-read instead of committed.
var errRetryIdempotency = errors.New("retry idempotency outcome")
