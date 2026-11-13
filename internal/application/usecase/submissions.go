package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// IdempotencyScopeSubmissionCreate namespaces keys of POST /submissions.
const IdempotencyScopeSubmissionCreate = "submissions.create"

// CreateSubmissionResult carries the stored proposal; Replayed is true when
// the Idempotency-Key already pinned an identical request.
type CreateSubmissionResult struct {
	Submission models.Submission
	Replayed   bool
}

type submissionCreator interface {
	Create(ctx context.Context, in models.CreateSubmissionInput) (models.Submission, error)
}

type submissionByKeyGetter interface {
	GetByAuthorAndKey(ctx context.Context, authorID, key string) (models.Submission, error)
}

type CreateSubmissionUseCase struct {
	submissions submissionCreator
	byKey       submissionByKeyGetter
	idem        idempotencyStore
	tx          txRunner
	ttl         time.Duration
}

func NewCreateSubmissionUseCase(
	submissions submissionCreator,
	byKey submissionByKeyGetter,
	idem idempotencyStore,
	tx txRunner,
	ttl time.Duration,
) *CreateSubmissionUseCase {
	return &CreateSubmissionUseCase{
		submissions: submissions,
		byKey:       byKey,
		idem:        idem,
		tx:          tx,
		ttl:         ttl,
	}
}

func (u *CreateSubmissionUseCase) Do(ctx context.Context, in models.CreateSubmissionInput) (CreateSubmissionResult, error) {
	if in.AuthorID == "" {
		return CreateSubmissionResult{}, models.ErrUnauthorized
	}
	if err := in.Validate(); err != nil {
		return CreateSubmissionResult{}, err
	}

	if in.IdempotencyKey != "" {
		stored, found, err := u.storedSubmission(ctx, in)
		if err != nil {
			return CreateSubmissionResult{}, err
		}
		if found {
			return CreateSubmissionResult{Submission: stored, Replayed: true}, nil
		}
	}

	result, err := u.create(ctx, in)
	if err == nil {
		return result, nil
	}

	if errors.Is(err, errRetryIdempotency) {
		stored, found, lookupErr := u.storedSubmission(ctx, in)
		if lookupErr != nil {
			return CreateSubmissionResult{}, lookupErr
		}
		if found {
			return CreateSubmissionResult{Submission: stored, Replayed: true}, nil
		}

		return CreateSubmissionResult{}, fmt.Errorf("%w: idempotency key race lost", models.ErrConflict)
	}

	return CreateSubmissionResult{}, err
}

func (u *CreateSubmissionUseCase) storedSubmission(
	ctx context.Context,
	in models.CreateSubmissionInput,
) (models.Submission, bool, error) {
	record, err := u.idem.Get(ctx, IdempotencyScopeSubmissionCreate, in.IdempotencyKey)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return models.Submission{}, false, nil
		}

		return models.Submission{}, false, fmt.Errorf("load idempotency record: %w", err)
	}

	if record.RequestFingerprint != in.RequestFingerprint {
		return models.Submission{}, false, fmt.Errorf(
			"%w: Idempotency-Key `%s` was already used with a different request body",
			models.ErrConflict, in.IdempotencyKey,
		)
	}

	stored, err := u.byKey.GetByAuthorAndKey(ctx, in.AuthorID, in.IdempotencyKey)
	if err != nil {
		return models.Submission{}, false, fmt.Errorf("load submission by idempotency record: %w", err)
	}

	return stored, true, nil
}

func (u *CreateSubmissionUseCase) create(
	ctx context.Context,
	in models.CreateSubmissionInput,
) (CreateSubmissionResult, error) {
	var result CreateSubmissionResult

	now := time.Now()

	err := u.tx.Run(ctx, func(txCtx context.Context) error {
		created, err := u.submissions.Create(txCtx, in)
		if err != nil {
			return fmt.Errorf("create submission: %w", err)
		}

		if in.IdempotencyKey != "" {
			record := models.IdempotencyRecord{
				Scope:              IdempotencyScopeSubmissionCreate,
				Key:                in.IdempotencyKey,
				RequestFingerprint: in.RequestFingerprint,
				UserID:             created.AuthorID,
				ExpiresAt:          now.Add(u.ttl),
			}

			inserted, err := u.idem.Save(txCtx, record)
			if err != nil {
				return fmt.Errorf("save idempotency record: %w", err)
			}
			if !inserted {
				return errRetryIdempotency
			}
		}

		result = CreateSubmissionResult{Submission: created}

		return nil
	})
	if err != nil {
		return CreateSubmissionResult{}, err
	}

	return result, nil
}

// GetSubmissionUseCase returns one of the author's own proposals.
type GetSubmissionUseCase struct {
	submissions submissionGetter
}

type submissionGetter interface {
	GetByIDAndAuthor(ctx context.Context, id, authorID string) (models.Submission, error)
}

func NewGetSubmissionUseCase(submissions submissionGetter) *GetSubmissionUseCase {
	return &GetSubmissionUseCase{submissions: submissions}
}

func (u *GetSubmissionUseCase) Do(ctx context.Context, id, authorID string) (models.Submission, error) {
	if authorID == "" {
		return models.Submission{}, models.ErrUnauthorized
	}

	validID, err := models.ValidateUUID("id", id)
	if err != nil {
		return models.Submission{}, err
	}

	return u.submissions.GetByIDAndAuthor(ctx, validID, authorID)
}

// ListSubmissionsUseCase returns the author's own proposals, newest first.
type ListSubmissionsUseCase struct {
	submissions submissionLister
}

type submissionLister interface {
	ListByAuthor(ctx context.Context, filter models.SubmissionFilter) ([]models.Submission, error)
}

func NewListSubmissionsUseCase(submissions submissionLister) *ListSubmissionsUseCase {
	return &ListSubmissionsUseCase{submissions: submissions}
}

func (u *ListSubmissionsUseCase) Do(ctx context.Context, filter models.SubmissionFilter) ([]models.Submission, error) {
	if filter.AuthorID == "" {
		return nil, models.ErrUnauthorized
	}

	userID, err := models.ValidateUUID("user_id", filter.AuthorID)
	if err != nil {
		return nil, err
	}
	filter.AuthorID = userID

	return u.submissions.ListByAuthor(ctx, filter)
}
