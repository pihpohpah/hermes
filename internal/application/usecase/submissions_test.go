package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pihpohpah/hermes/internal/application/usecase"
	"github.com/pihpohpah/hermes/internal/domain/models"
)

type fakeSubmissionStore struct {
	byAuthorKey map[string]models.Submission
	created     int
}

func (f *fakeSubmissionStore) Create(_ context.Context, in models.CreateSubmissionInput) (models.Submission, error) {
	f.created++

	return models.Submission{
		ID:       "sub-1",
		AuthorID: in.AuthorID,
		Type:     in.Type,
		Title:    in.Title,
		Status:   models.SubmissionStatusReview,
	}, nil
}

func (f *fakeSubmissionStore) GetByAuthorAndKey(_ context.Context, authorID, key string) (models.Submission, error) {
	submission, ok := f.byAuthorKey[authorID+"/"+key]
	if !ok {
		return models.Submission{}, models.ErrNotFound
	}

	return submission, nil
}

type fakeIdempotency struct {
	records map[string]models.IdempotencyRecord
}

func (f *fakeIdempotency) Get(_ context.Context, scope, key string) (models.IdempotencyRecord, error) {
	record, ok := f.records[scope+"/"+key]
	if !ok {
		return models.IdempotencyRecord{}, models.ErrNotFound
	}

	return record, nil
}

func (f *fakeIdempotency) Save(_ context.Context, record models.IdempotencyRecord) (bool, error) {
	id := record.Scope + "/" + record.Key
	if _, exists := f.records[id]; exists {
		return false, nil
	}
	f.records[id] = record

	return true, nil
}

type directTx struct{}

func (directTx) Run(_ context.Context, fn func(context.Context) error) error {
	return fn(context.Background())
}

func validSubmissionInput() models.CreateSubmissionInput {
	return models.CreateSubmissionInput{
		Type:     models.SubmissionTypePlace,
		Title:    "Новая смотровая на сопке",
		AuthorID: "00000000-0000-0000-0000-000000000001",
	}
}

func newCreateSubmissionUC(
	store *fakeSubmissionStore,
	idem *fakeIdempotency,
) *usecase.CreateSubmissionUseCase {
	return usecase.NewCreateSubmissionUseCase(store, store, idem, directTx{}, time.Hour)
}

func TestCreateSubmissionHappyPath(t *testing.T) {
	store := &fakeSubmissionStore{byAuthorKey: map[string]models.Submission{}}
	uc := newCreateSubmissionUC(store, &fakeIdempotency{records: map[string]models.IdempotencyRecord{}})

	result, err := uc.Do(context.Background(), validSubmissionInput())
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result.Submission.Status != models.SubmissionStatusReview {
		t.Fatalf("status = %q, want review", result.Submission.Status)
	}
	if result.Replayed {
		t.Fatal("fresh create reported as replayed")
	}
	if store.created != 1 {
		t.Fatalf("created = %d, want 1", store.created)
	}
}

func TestCreateSubmissionIdempotentReplay(t *testing.T) {
	store := &fakeSubmissionStore{byAuthorKey: map[string]models.Submission{}}
	idem := &fakeIdempotency{records: map[string]models.IdempotencyRecord{}}
	uc := newCreateSubmissionUC(store, idem)

	in := validSubmissionInput()
	in.IdempotencyKey = "key-1"
	in.RequestFingerprint = "fp-1"

	if _, err := uc.Do(context.Background(), in); err != nil {
		t.Fatalf("first Do() error = %v", err)
	}

	store.byAuthorKey[in.AuthorID+"/key-1"] = models.Submission{
		ID: "sub-1", AuthorID: in.AuthorID, Status: models.SubmissionStatusReview,
	}

	result, err := uc.Do(context.Background(), in)
	if err != nil {
		t.Fatalf("replay Do() error = %v", err)
	}
	if !result.Replayed {
		t.Fatal("identical retry not reported as replayed")
	}
	if store.created != 1 {
		t.Fatalf("created = %d, want 1 (no duplicate)", store.created)
	}
}

func TestCreateSubmissionFingerprintConflict(t *testing.T) {
	store := &fakeSubmissionStore{byAuthorKey: map[string]models.Submission{}}
	idem := &fakeIdempotency{records: map[string]models.IdempotencyRecord{}}
	uc := newCreateSubmissionUC(store, idem)

	in := validSubmissionInput()
	in.IdempotencyKey = "key-1"
	in.RequestFingerprint = "fp-1"
	if _, err := uc.Do(context.Background(), in); err != nil {
		t.Fatalf("first Do() error = %v", err)
	}

	in.Title = "Другая смотровая"
	in.RequestFingerprint = "fp-2"
	_, err := uc.Do(context.Background(), in)
	if !errors.Is(err, models.ErrConflict) {
		t.Fatalf("same key with different body: error = %v, want ErrConflict", err)
	}
}
