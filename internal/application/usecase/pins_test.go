package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

type fakePinStore struct {
	active  int
	recent  int
	created models.CreatePinInput
}

func (f *fakePinStore) Create(_ context.Context, in models.CreatePinInput) (models.Pin, error) {
	f.created = in

	return models.Pin{ID: "pin-1", AuthorID: in.AuthorID}, nil
}

func (f *fakePinStore) CountActiveByAuthor(_ context.Context, _ string, _ time.Time) (int, error) {
	return f.active, nil
}

func (f *fakePinStore) CountRecentByAuthor(_ context.Context, _ string, _ time.Time) (int, error) {
	return f.recent, nil
}

func validCreateInput() models.CreatePinInput {
	starts := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

	return models.CreatePinInput{
		Kind:     models.PinKindWalk,
		Title:    "Идём на закат к маяку",
		Location: models.Point{Lat: 43.08, Lon: 131.86},
		StartsAt: starts,
		EndsAt:   starts.Add(time.Hour),
		AuthorID: "00000000-0000-0000-0000-000000000001",
	}
}

func TestCreatePinAppliesStopWords(t *testing.T) {
	t.Parallel()

	store := &fakePinStore{}
	uc := NewCreatePinUseCase(store, PinRules{StopWords: []string{"ставки", "КРИПТА"}}, time.Now)

	in := validCreateInput()
	in.Title = "Крипта ставки бесплатно"
	if _, err := uc.Do(context.Background(), in); err == nil {
		t.Fatal("expected stop word rejection")
	} else if !errors.Is(err, models.ErrInvalidArgument) {
		t.Fatalf("want ErrInvalidArgument, got %v", err)
	}
}

func TestCreatePinEnforcesActiveLimit(t *testing.T) {
	t.Parallel()

	store := &fakePinStore{active: 5}
	uc := NewCreatePinUseCase(store, PinRules{MaxActivePerUser: 5}, time.Now)

	if _, err := uc.Do(context.Background(), validCreateInput()); err == nil {
		t.Fatal("expected active pin limit rejection")
	}
}

func TestCreatePinEnforcesHourlyRateLimit(t *testing.T) {
	t.Parallel()

	store := &fakePinStore{recent: 10}
	uc := NewCreatePinUseCase(store, PinRules{MaxPerHour: 10}, time.Now)

	if _, err := uc.Do(context.Background(), validCreateInput()); err == nil {
		t.Fatal("expected hourly rate limit rejection")
	}
}

func TestCreatePinNightBan(t *testing.T) {
	t.Parallel()

	vladivostok, err := time.LoadLocation("Asia/Vladivostok")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}

	// 02:30 local — inside the 01:00–07:00 night window.
	night := time.Date(2026, 11, 14, 2, 30, 0, 0, vladivostok)
	store := &fakePinStore{}
	uc := NewCreatePinUseCase(
		store,
		PinRules{
			NightBanFrom: "01:00",
			NightBanTo:   "07:00",
			Location:     vladivostok,
		},
		func() time.Time { return night },
	)

	if _, err := uc.Do(context.Background(), validCreateInput()); err == nil {
		t.Fatal("expected night ban rejection at 02:30 local")
	}

	// 12:00 local — outside the window: the same input passes.
	day := time.Date(2026, 11, 14, 12, 0, 0, 0, vladivostok)
	uc = NewCreatePinUseCase(
		store,
		PinRules{
			NightBanFrom: "01:00",
			NightBanTo:   "07:00",
			Location:     vladivostok,
		},
		func() time.Time { return day },
	)

	if _, err := uc.Do(context.Background(), validCreateInput()); err != nil {
		t.Fatalf("daytime creation rejected: %v", err)
	}
	if store.created.AuthorID != "00000000-0000-0000-0000-000000000001" {
		t.Fatalf("author not passed through: %q", store.created.AuthorID)
	}
}

func TestCreatePinTrimsAndNormalizes(t *testing.T) {
	t.Parallel()

	store := &fakePinStore{}
	uc := NewCreatePinUseCase(store, PinRules{}, time.Now)

	in := validCreateInput()
	in.Title = "  Идём на закат к маяку  "
	in.Note = " берите термосы "
	in.PlaceLabel = "  Поле у стадиона "

	if _, err := uc.Do(context.Background(), in); err != nil {
		t.Fatalf("creation rejected: %v", err)
	}
	if store.created.Title != "Идём на закат к маяку" {
		t.Fatalf("title not trimmed: %q", store.created.Title)
	}
	if store.created.Note != "берите термосы" {
		t.Fatalf("note not trimmed: %q", store.created.Note)
	}
}
