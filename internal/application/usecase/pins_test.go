package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pihpohpah/hermes/internal/application/usecase"
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

func validCreatePinInput() models.CreatePinInput {
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
	store := &fakePinStore{}
	uc := usecase.NewCreatePinUseCase(store, usecase.PinRules{StopWords: []string{"ставки", "КРИПТА"}}, time.Now)

	in := validCreatePinInput()
	in.Title = "Крипта ставки бесплатно"
	if _, err := uc.Do(context.Background(), in); err == nil {
		t.Fatal("expected stop word rejection")
	} else if !errors.Is(err, models.ErrInvalidArgument) {
		t.Fatalf("want ErrInvalidArgument, got %v", err)
	}
}

func TestCreatePinEnforcesLimits(t *testing.T) {
	tests := []struct {
		name  string
		store *fakePinStore
		rules usecase.PinRules
	}{
		{name: "active pin cap", store: &fakePinStore{active: 5}, rules: usecase.PinRules{MaxActivePerUser: 5}},
		{name: "hourly rate limit", store: &fakePinStore{recent: 10}, rules: usecase.PinRules{MaxPerHour: 10}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := usecase.NewCreatePinUseCase(tt.store, tt.rules, time.Now)
			if _, err := uc.Do(context.Background(), validCreatePinInput()); err == nil {
				t.Fatal("expected moderation rejection")
			}
		})
	}
}

func TestCreatePinNightBan(t *testing.T) {
	vladivostok, err := time.LoadLocation("Asia/Vladivostok")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}

	rules := usecase.PinRules{
		NightBanFrom: "01:00",
		NightBanTo:   "07:00",
		Location:     vladivostok,
	}

	night := time.Date(2026, 11, 14, 2, 30, 0, 0, vladivostok)
	nightUC := usecase.NewCreatePinUseCase(&fakePinStore{}, rules, func() time.Time { return night })
	if _, err := nightUC.Do(context.Background(), validCreatePinInput()); err == nil {
		t.Fatal("expected night ban rejection at 02:30 local")
	}

	day := time.Date(2026, 11, 14, 12, 0, 0, 0, vladivostok)
	store := &fakePinStore{}
	dayUC := usecase.NewCreatePinUseCase(store, rules, func() time.Time { return day })
	if _, err := dayUC.Do(context.Background(), validCreatePinInput()); err != nil {
		t.Fatalf("daytime creation rejected: %v", err)
	}
	if store.created.AuthorID != "00000000-0000-0000-0000-000000000001" {
		t.Fatalf("author not passed through: %q", store.created.AuthorID)
	}
}

func TestCreatePinTrimsAndNormalizes(t *testing.T) {
	store := &fakePinStore{}
	uc := usecase.NewCreatePinUseCase(store, usecase.PinRules{}, time.Now)

	in := validCreatePinInput()
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
