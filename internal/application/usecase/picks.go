package usecase

import (
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

const (
	defaultPickLimit = 20
	maxPickLimit     = 50
	// pickGoodScore is the "comfortable hour" threshold for the day summary
	// (score >= 70 counts as a good hour in the picker's «хороших часов»).
	pickGoodScore = 70
)

// PickTripsUseCase answers "куда поехать по погоде": for one activity and
// calendar day it ranks places by their best cached hourly score. The day is
// interpreted in Asia/Vladivostok and clamped to the 10-day forecast horizon;
// past hours of today are excluded, so plans always look forward.
type PickTripsUseCase struct {
	picks dayPickLister
	now   func() time.Time
}

type dayPickLister interface {
	ListDayPicks(
		ctx context.Context,
		activity models.Activity,
		from time.Time,
		to time.Time,
		goodScore int,
		limit int,
	) ([]models.ConditionPick, error)
}

func NewPickTripsUseCase(picks dayPickLister, now func() time.Time) *PickTripsUseCase {
	return &PickTripsUseCase{picks: picks, now: now}
}

func (u *PickTripsUseCase) Do(ctx context.Context, in models.PickQuery) ([]models.ConditionPick, error) {
	activity := in.Activity
	if activity == "" {
		activity = models.ActivityWalk
	}
	if !activity.Valid() {
		return nil, models.Invalid("unexpected activity `" + string(activity) + "`")
	}

	now := u.now()
	today, err := models.CurrentDayWindow(now)
	if err != nil {
		return nil, err
	}

	window := today
	if !in.Date.IsZero() {
		window, err = models.CurrentDayWindow(in.Date)
		if err != nil {
			return nil, err
		}
	}

	lastAllowed := today.Day.AddDate(0, 0, models.ForecastHorizonDays-1)
	if window.Day.After(lastAllowed) {
		return nil, models.Invalid(
			"date is beyond the " + strconv.Itoa(models.ForecastHorizonDays) + "-day forecast horizon",
		)
	}

	from := window.Start
	if floor := now.UTC().Truncate(time.Hour); from.Before(floor) {
		from = floor
	}

	limit := in.Limit
	if limit <= 0 {
		limit = defaultPickLimit
	}
	limit = min(limit, maxPickLimit)

	picks, err := u.picks.ListDayPicks(ctx, activity, from, window.End, pickGoodScore, limit)
	if err != nil {
		return nil, err
	}

	sort.SliceStable(picks, func(i, j int) bool {
		if picks[i].Score != picks[j].Score {
			return picks[i].Score > picks[j].Score
		}
		if picks[i].GoodHours != picks[j].GoodHours {
			return picks[i].GoodHours > picks[j].GoodHours
		}

		return picks[i].ForecastTime.Before(picks[j].ForecastTime)
	})

	return picks, nil
}
