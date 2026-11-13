package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// PinRules is the moderation policy of the radar. It is the backend half of
// the open questions from the design TODO: per-user limits, rate limiting,
// stop words and night hours. Zero values disable the corresponding rule.
type PinRules struct {
	// MaxActivePerUser caps the number of live pins one author may hold.
	MaxActivePerUser int
	// MaxPerHour is the rate limit: creations in the trailing hour.
	MaxPerHour int
	// StopWords rejects pins whose title or note contains any of these words.
	StopWords []string
	// NightBanFrom/NightBanTo (local "HH:MM") reject creation during the
	// night window. From > To means the window crosses midnight. Empty From
	// disables the rule.
	NightBanFrom string
	NightBanTo   string
	// ReportHideThreshold hides a pin after that many distinct complaints.
	ReportHideThreshold int
	// MaxDuration caps a pin's lifetime from start to end.
	MaxDuration time.Duration
	// Location is the timezone the night window is evaluated in.
	Location *time.Location
}

func (r PinRules) location() *time.Location {
	if r.Location != nil {
		return r.Location
	}

	return time.UTC
}

// validatePinID pins have no slugs: the path id must be a UUID, otherwise a
// malformed id would surface as a database error instead of a clean 400.
func validatePinID(raw string) (string, error) {
	return models.ValidateUUID("id", raw)
}

// ListPinsUseCase returns the active pins in a viewport, decorated for the
// viewer when the request is authenticated.
type ListPinsUseCase struct {
	pins pinLister
	now  func() time.Time
}

type pinLister interface {
	ListActive(ctx context.Context, filter models.PinFilter) ([]models.Pin, error)
}

func NewListPinsUseCase(pins pinLister, now func() time.Time) *ListPinsUseCase {
	return &ListPinsUseCase{pins: pins, now: now}
}

func (u *ListPinsUseCase) Do(ctx context.Context, filter models.PinFilter) ([]models.Pin, error) {
	filter.Now = u.now()

	return u.pins.ListActive(ctx, filter)
}

// GetPinUseCase loads one visible pin.
type GetPinUseCase struct {
	pins pinGetter
	now  func() time.Time
}

type pinGetter interface {
	GetVisible(ctx context.Context, id string, viewer *string, now time.Time) (models.Pin, error)
}

func NewGetPinUseCase(pins pinGetter, now func() time.Time) *GetPinUseCase {
	return &GetPinUseCase{pins: pins, now: now}
}

func (u *GetPinUseCase) Do(ctx context.Context, id string, viewer *string) (models.Pin, error) {
	validID, err := validatePinID(id)
	if err != nil {
		return models.Pin{}, err
	}

	return u.pins.GetVisible(ctx, validID, viewer, u.now())
}

// CreatePinUseCase validates shape and moderation policy, then stores the pin.
type CreatePinUseCase struct {
	pins  pinCreator
	rules PinRules
	now   func() time.Time
}

type pinCreator interface {
	Create(ctx context.Context, in models.CreatePinInput) (models.Pin, error)
	CountActiveByAuthor(ctx context.Context, authorID string, now time.Time) (int, error)
	CountRecentByAuthor(ctx context.Context, authorID string, since time.Time) (int, error)
}

func NewCreatePinUseCase(pins pinCreator, rules PinRules, now func() time.Time) *CreatePinUseCase {
	return &CreatePinUseCase{pins: pins, rules: rules, now: now}
}

func (u *CreatePinUseCase) Do(ctx context.Context, in models.CreatePinInput) (models.Pin, error) {
	if in.AuthorID == "" {
		return models.Pin{}, models.ErrUnauthorized
	}

	maxDuration := u.rules.MaxDuration
	if maxDuration <= 0 {
		maxDuration = defaultPinMaxDuration
	}
	if err := in.Validate(maxDuration); err != nil {
		return models.Pin{}, err
	}

	now := u.now()
	if err := u.checkModeration(ctx, in, now); err != nil {
		return models.Pin{}, err
	}

	in.Title = strings.TrimSpace(in.Title)
	in.Note = strings.TrimSpace(in.Note)
	in.PlaceLabel = strings.TrimSpace(in.PlaceLabel)
	in.StartsAt = in.StartsAt.UTC()
	in.EndsAt = in.EndsAt.UTC()

	return u.pins.Create(ctx, in)
}

func (u *CreatePinUseCase) checkModeration(ctx context.Context, in models.CreatePinInput, now time.Time) error {
	if u.rules.MaxActivePerUser > 0 {
		active, err := u.pins.CountActiveByAuthor(ctx, in.AuthorID, now)
		if err != nil {
			return err
		}
		if active >= u.rules.MaxActivePerUser {
			return models.Invalidf(
				"слишком много живых меток: максимум %d, удалите старые и попробуйте снова",
				u.rules.MaxActivePerUser,
			)
		}
	}

	if u.rules.MaxPerHour > 0 {
		recent, err := u.pins.CountRecentByAuthor(ctx, in.AuthorID, now.Add(-time.Hour))
		if err != nil {
			return err
		}
		if recent >= u.rules.MaxPerHour {
			return models.Invalidf(
				"слишком много меток за час: максимум %d, попробуйте позже",
				u.rules.MaxPerHour,
			)
		}
	}

	if len(u.rules.StopWords) > 0 {
		haystack := strings.ToLower(in.Title + " " + in.Note)
		for _, word := range u.rules.StopWords {
			if word == "" {
				continue
			}
			if strings.Contains(haystack, strings.ToLower(word)) {
				return models.Invalid("метка не прошла автоматическую проверку, переформулируйте текст")
			}
		}
	}

	if err := u.checkNightBan(now); err != nil {
		return err
	}

	return nil
}

// checkNightBan rejects creation inside the configured local night window.
func (u *CreatePinUseCase) checkNightBan(now time.Time) error {
	if u.rules.NightBanFrom == "" || u.rules.NightBanTo == "" {
		return nil
	}

	localNow := now.In(u.rules.location())
	minutes := localNow.Hour()*60 + localNow.Minute()

	from, err := dayMinutes(u.rules.NightBanFrom)
	if err != nil {
		return err
	}
	to, err := dayMinutes(u.rules.NightBanTo)
	if err != nil {
		return err
	}

	inside := from <= to && minutes >= from && minutes < to
	overnight := from > to && (minutes >= from || minutes < to)
	if inside || overnight {
		return models.Invalid("метки временно недоступны в ночные часы, попробуйте утром")
	}

	return nil
}

// dayMinutes parses "HH:MM" into minutes since local midnight.
func dayMinutes(raw string) (int, error) {
	var hours, minutes int
	if _, err := fmt.Sscanf(raw, "%d:%d", &hours, &minutes); err != nil {
		return 0, fmt.Errorf("invalid time of day `%s`: %w", raw, err)
	}
	if hours < 0 || hours > 23 || minutes < 0 || minutes > 59 {
		return 0, fmt.Errorf("invalid time of day `%s`", raw)
	}

	return hours*60 + minutes, nil
}

const defaultPinMaxDuration = 12 * time.Hour

// DeletePinUseCase removes the author's own pin.
type DeletePinUseCase struct {
	pins pinDeleter
}

type pinDeleter interface {
	DeleteByAuthor(ctx context.Context, id, authorID string) error
}

func NewDeletePinUseCase(pins pinDeleter) *DeletePinUseCase {
	return &DeletePinUseCase{pins: pins}
}

func (u *DeletePinUseCase) Do(ctx context.Context, id, authorID string) error {
	if authorID == "" {
		return models.ErrUnauthorized
	}

	validID, err := validatePinID(id)
	if err != nil {
		return err
	}

	return u.pins.DeleteByAuthor(ctx, validID, authorID)
}

// SetPinGoingUseCase joins or leaves a pin. The pin must still be live.
type SetPinGoingUseCase struct {
	pins pinGoingSetter
	now  func() time.Time
}

type pinGoingSetter interface {
	GetVisible(ctx context.Context, id string, viewer *string, now time.Time) (models.Pin, error)
	SetGoing(ctx context.Context, in models.SetGoingInput) error
}

func NewSetPinGoingUseCase(pins pinGoingSetter, now func() time.Time) *SetPinGoingUseCase {
	return &SetPinGoingUseCase{pins: pins, now: now}
}

func (u *SetPinGoingUseCase) Do(ctx context.Context, in models.SetGoingInput) (models.Pin, error) {
	if in.UserID == "" {
		return models.Pin{}, models.ErrUnauthorized
	}

	validID, err := validatePinID(in.PinID)
	if err != nil {
		return models.Pin{}, err
	}
	in.PinID = validID

	in.Now = u.now()
	if _, err := u.pins.GetVisible(ctx, in.PinID, &in.UserID, in.Now); err != nil {
		return models.Pin{}, err
	}

	if err := u.pins.SetGoing(ctx, in); err != nil {
		return models.Pin{}, err
	}

	return u.pins.GetVisible(ctx, in.PinID, &in.UserID, in.Now)
}

// ReportPinUseCase stores a complaint and applies the hide threshold.
type ReportPinUseCase struct {
	pins  pinReporter
	rules PinRules
	now   func() time.Time
}

type pinReporter interface {
	GetVisible(ctx context.Context, id string, viewer *string, now time.Time) (models.Pin, error)
	CreateReport(ctx context.Context, in models.CreatePinReportInput, hideThreshold int) (models.PinReport, error)
}

func NewReportPinUseCase(pins pinReporter, rules PinRules, now func() time.Time) *ReportPinUseCase {
	return &ReportPinUseCase{pins: pins, rules: rules, now: now}
}

func (u *ReportPinUseCase) Do(ctx context.Context, in models.CreatePinReportInput) (models.PinReport, error) {
	if in.UserID == "" {
		return models.PinReport{}, models.ErrUnauthorized
	}
	if err := in.Validate(); err != nil {
		return models.PinReport{}, err
	}

	validID, err := validatePinID(in.PinID)
	if err != nil {
		return models.PinReport{}, err
	}
	in.PinID = validID

	in.Now = u.now()
	if _, err := u.pins.GetVisible(ctx, in.PinID, &in.UserID, in.Now); err != nil {
		return models.PinReport{}, err
	}

	in.Reason = strings.TrimSpace(in.Reason)

	return u.pins.CreateReport(ctx, in, u.rules.ReportHideThreshold)
}

// PurgeExpiredPinsUseCase physically removes pins past their grace period.
type PurgeExpiredPinsUseCase struct {
	pins pinPurger
}

type pinPurger interface {
	PurgeExpired(ctx context.Context, grace time.Duration) (int64, error)
}

func NewPurgeExpiredPinsUseCase(pins pinPurger) *PurgeExpiredPinsUseCase {
	return &PurgeExpiredPinsUseCase{pins: pins}
}

func (u *PurgeExpiredPinsUseCase) Do(ctx context.Context, grace time.Duration) (int64, error) {
	if grace <= 0 {
		grace = defaultPinPurgeGrace
	}

	return u.pins.PurgeExpired(ctx, grace)
}

const defaultPinPurgeGrace = 24 * time.Hour
