package models

import "fmt"

// Activity is the kind of outdoor pursuit a forecast is scored for.
type Activity string

const (
	ActivityHike Activity = "hike"
	ActivitySup  Activity = "sup"
	ActivityRun  Activity = "run"
	ActivityWalk Activity = "walk"
)

// NewActivity parses an activity coming from an HTTP query, CLI flag or a
// database row. Unlike a plain type conversion it rejects unknown values, so a
// typo cannot silently produce a zero-score forecast.
func NewActivity(raw string) (Activity, error) {
	activity := Activity(raw)
	if !activity.Valid() {
		return "", Invalid(fmt.Sprintf(
			"unexpected activity `%s`, expected: hike, sup, run, walk", raw,
		))
	}

	return activity, nil
}
