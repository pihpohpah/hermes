package models

import (
	"fmt"
	"time"
)

const (
	minConditionScore = 0
	maxConditionScore = 100
)

// Score is the comfortable-conditions score, 0..100. It says nothing about
// safety: always read it together with Verdict and Warnings.
type Score int

func NewScore(raw int) (Score, error) {
	if raw < minConditionScore || raw > maxConditionScore {
		return 0, Invalid(fmt.Sprintf("score `%d` must be between 0 and 100", raw))
	}

	return Score(raw), nil
}

// Factor identifies an input of the condition scorer.
type Factor string

const (
	FactorWindSpeed           Factor = "wind_speed"
	FactorWindGust            Factor = "wind_gust"
	FactorWaveHeight          Factor = "wave_height"
	FactorPrecipitation       Factor = "precipitation"
	FactorAirQuality          Factor = "air_quality"
	FactorWaterTemperature    Factor = "water_temperature"
	FactorAirTemperature      Factor = "air_temperature"
	FactorApparentTemperature Factor = "apparent_temperature"
	FactorVisibility          Factor = "visibility"
	FactorWeatherCode         Factor = "weather_code"
)

// RiskLevel is the severity of a separate safety warning.
type RiskLevel string

const (
	RiskInfo    RiskLevel = "info"
	RiskCaution RiskLevel = "caution"
	RiskDanger  RiskLevel = "danger"
)

// Verdict is the only place where a recommendation is expressed. A high
// comfort score never implies a "go" verdict by itself.
type Verdict string

const (
	VerdictGo      Verdict = "go"
	VerdictCaution Verdict = "caution"
	VerdictNoGo    Verdict = "no_go"
)

type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// Contribution explains how one factor moved the comfort score.
type Contribution struct {
	Factor  Factor  `json:"factor"`
	Value   Measure `json:"value"`
	Penalty float64 `json:"penalty"`
	Note    string  `json:"note,omitempty"`
}

// Warning is a safety statement decoupled from the comfort score.
type Warning struct {
	Code    string    `json:"code"`
	Level   RiskLevel `json:"level"`
	Factor  Factor    `json:"factor"`
	Value   Measure   `json:"value"`
	Message string    `json:"message"`
}

// ConditionAssessment is the scorer output for a place, activity and hour.
// Comfort (Score) and safety (Warnings/Verdict) are deliberately separate.
type ConditionAssessment struct {
	PlaceID          string
	Activity         Activity
	ForecastTime     time.Time
	LocalDate        string
	Observation      WeatherObservation
	Geometry         ModelGeometry
	Score            Score
	Verdict          Verdict
	Confidence       Confidence
	Limitations      []string
	Summary          string
	Contributions    []Contribution
	Warnings         []Warning
	Sources          []SourceStatus
	Coverage         []SourceCoverage
	Demo             bool
	AlgorithmVersion string
}

// ConditionScore is kept as the compatibility alias used by repositories and
// presenters for a persisted assessment.
type ConditionScore = ConditionAssessment

// ConditionPick is the best-scored hour of one place within a calendar day
// plus the number of comfortable hours — the payload of the trip picker that
// answers "куда поехать по погоде".
type ConditionPick struct {
	PlaceID      string
	PlaceName    string
	PlaceSlug    string
	Scenario     string
	Category     string
	District     string
	Score        Score
	Verdict      Verdict
	ForecastTime time.Time
	Summary      string
	GoodHours    int
	TotalHours   int
}

// PickQuery is the parsed input of the picker read use case.
type PickQuery struct {
	Activity Activity
	Date     time.Time
	Limit    int
}

func NewActivityFromPlaceType(pt PlaceType) Activity {
	switch pt {
	case PlaceTypeTrailhead:
		return ActivityHike
	case PlaceTypeSupSpot:
		return ActivitySup
	case PlaceTypeBeach, PlaceTypePark, PlaceTypeViewpoint:
		return ActivityWalk
	default:
		return ActivityWalk
	}
}

// ForecastQuery is the parsed input of the forecast read use case. Days selects
// how many calendar days to return, starting from Date.
type ForecastQuery struct {
	PlaceID  string
	Date     time.Time
	Activity Activity
	Days     int
}

func (a Activity) Valid() bool {
	switch a {
	case ActivityHike, ActivitySup, ActivityRun, ActivityWalk:
		return true
	default:
		return false
	}
}

func NewVerdict(raw string) (Verdict, error) {
	switch Verdict(raw) {
	case VerdictGo:
		return VerdictGo, nil
	case VerdictCaution:
		return VerdictCaution, nil
	case VerdictNoGo:
		return VerdictNoGo, nil
	default:
		return "", Invalid(fmt.Sprintf("unexpected verdict `%s`, expected: go, caution, no_go", raw))
	}
}

func (v Verdict) Valid() bool {
	switch v {
	case VerdictGo, VerdictCaution, VerdictNoGo:
		return true
	default:
		return false
	}
}

func NewConfidence(raw string) (Confidence, error) {
	switch Confidence(raw) {
	case ConfidenceHigh:
		return ConfidenceHigh, nil
	case ConfidenceMedium:
		return ConfidenceMedium, nil
	case ConfidenceLow:
		return ConfidenceLow, nil
	default:
		return "", Invalid(fmt.Sprintf("unexpected confidence `%s`, expected: high, medium, low", raw))
	}
}

func (c Confidence) Valid() bool {
	switch c {
	case ConfidenceHigh, ConfidenceMedium, ConfidenceLow:
		return true
	default:
		return false
	}
}
