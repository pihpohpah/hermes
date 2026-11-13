package scoring_test

import (
	"strings"
	"testing"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
	"github.com/pihpohpah/hermes/internal/domain/scoring"
)

func testHour() time.Time {
	return time.Date(2026, 11, 13, 2, 0, 0, 0, time.UTC)
}

func known(v float64) models.Measure { return models.NewMeasure(v) }

func unknown() models.Measure { return models.MissingMeasure() }

func observation(mutators ...func(*models.WeatherObservation)) models.WeatherObservation {
	obs := models.WeatherObservation{
		Hour:          testHour(),
		AirTempC:      known(18),
		ApparentTempC: known(17),
		PrecipMM:      known(0),
		WindSpeedMS:   known(2),
		WindGustMS:    known(4),
		WindDirection: known(180),
		HumidityPct:   known(60),
		CloudCoverPct: known(20),
		VisibilityM:   known(20000),
		WeatherCode:   known(1),
		WaveHeightM:   known(0.2),
		WavePeriodS:   known(4),
		WaveDirection: known(180),
		WaterTempC:    known(19),
		AQIEuropean:   known(20),
		PM25:          known(7),
	}
	for _, mutate := range mutators {
		mutate(&obs)
	}

	return obs
}

func evaluate(
	t *testing.T,
	activity models.Activity,
	obs models.WeatherObservation,
) models.ConditionAssessment {
	t.Helper()

	assessment, err := scoring.Evaluate(activity, "place-1", obs, nil, false)
	if err != nil {
		t.Fatalf("evaluate %s: %v", activity, err)
	}

	return assessment
}

// Table 1: identical weather must produce different, explainable verdicts for
// a walk and a SUP session.
func TestSameWeatherDiffersByActivity(t *testing.T) {
	t.Parallel()

	weather := observation(func(o *models.WeatherObservation) {
		o.WindSpeedMS = known(7)
		o.WindGustMS = known(10)
		o.WaveHeightM = known(0.8)
		o.WaterTempC = known(13)
		o.AirTempC = known(15)
	})

	walk := evaluate(t, models.ActivityWalk, weather)
	sup := evaluate(t, models.ActivitySup, weather)

	if walk.Score <= sup.Score {
		t.Fatalf("expected walk score %d > sup score %d", walk.Score, sup.Score)
	}
	if walk.Verdict == models.VerdictNoGo {
		t.Fatalf("walk should stay usable in these conditions, got %s", walk.Verdict)
	}
	if sup.Verdict == models.VerdictGo {
		t.Fatalf("sup must not be recommended with 0.8 m waves and 7 m/s wind, got %s", sup.Verdict)
	}
	if len(sup.Warnings) <= len(walk.Warnings) {
		t.Fatalf("sup should have more safety warnings than walk: %d vs %d", len(sup.Warnings), len(walk.Warnings))
	}
	if !hasWarning(sup, "wave") && !hasWarning(sup, "cold_water") {
		t.Fatalf("sup warnings must mention wave or cold water, got %+v", sup.Warnings)
	}
	if walk.Summary == sup.Summary {
		t.Fatalf("explanations must differ per activity, both were %q", walk.Summary)
	}
	if len(sup.Contributions) == 0 {
		t.Fatal("assessment must contain per-factor contributions")
	}
	for _, contribution := range sup.Contributions {
		if contribution.Penalty < 0 {
			t.Fatalf("penalty cannot be negative: %+v", contribution)
		}
	}
}

// Table 2: critical data gaps must never produce a green recommendation and
// must never be interpreted as calm weather.
func TestMissingCriticalDataNeverGreen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		activity models.Activity
		mutate   func(*models.WeatherObservation)
		wantCode string
	}{
		{
			name:     "sup without wave height",
			activity: models.ActivitySup,
			mutate:   func(o *models.WeatherObservation) { o.WaveHeightM = unknown() },
			wantCode: "missing_wave_height",
		},
		{
			name:     "sup without water temperature",
			activity: models.ActivitySup,
			mutate:   func(o *models.WeatherObservation) { o.WaterTempC = unknown() },
			wantCode: "missing_water_temperature",
		},
		{
			name:     "walk without wind",
			activity: models.ActivityWalk,
			mutate:   func(o *models.WeatherObservation) { o.WindSpeedMS = unknown() },
			wantCode: "missing_wind_speed",
		},
		{
			name:     "hike without air temperature",
			activity: models.ActivityHike,
			mutate:   func(o *models.WeatherObservation) { o.AirTempC = unknown() },
			wantCode: "missing_air_temperature",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assessment := evaluate(t, tc.activity, observation(tc.mutate))

			if assessment.Verdict == models.VerdictGo {
				t.Fatalf("missing critical data must not yield %q verdict", assessment.Verdict)
			}
			if int(assessment.Score) > 55 {
				t.Fatalf("comfort score must be capped when critical data is missing, got %d", assessment.Score)
			}
			if assessment.Confidence != models.ConfidenceLow {
				t.Fatalf("confidence must be low, got %s", assessment.Confidence)
			}
			if !hasWarning(assessment, tc.wantCode) {
				t.Fatalf("expected warning %q, got %+v", tc.wantCode, assessment.Warnings)
			}
			if len(assessment.Limitations) == 0 {
				t.Fatal("missing data must be explained in limitations")
			}
		})
	}
}

// The absence of marine data must never read as flat water.
func TestUnknownWaveIsNotCalm(t *testing.T) {
	t.Parallel()

	withWave := evaluate(t, models.ActivitySup, observation())
	withoutWave := evaluate(t, models.ActivitySup, observation(func(o *models.WeatherObservation) {
		o.WaveHeightM = unknown()
	}))

	if withoutWave.Verdict == models.VerdictGo {
		t.Fatal("unknown wave height must not produce a go verdict")
	}
	if withoutWave.Score >= withWave.Score {
		t.Fatalf("unknown wave must not score better than known calm water: %d vs %d",
			withoutWave.Score, withWave.Score)
	}

	warning, ok := findWarning(withoutWave, "missing_wave_height")
	if !ok {
		t.Fatalf("expected missing_wave_height warning, got %+v", withoutWave.Warnings)
	}
	if warning.Value.Known {
		t.Fatal("warning value must stay explicitly unknown")
	}
	if warning.Message == "" {
		t.Fatal("warning must explain itself")
	}
}

// Table 3: score boundaries and verdict gates.
func TestScoreBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		activity    models.Activity
		obs         models.WeatherObservation
		wantMin     int
		wantMax     int
		wantVerdict models.Verdict
	}{
		{
			name:        "ideal walk",
			activity:    models.ActivityWalk,
			obs:         observation(),
			wantMin:     95,
			wantMax:     100,
			wantVerdict: models.VerdictGo,
		},
		{
			name:     "moderate wind walk stays usable",
			activity: models.ActivityWalk,
			obs: observation(func(o *models.WeatherObservation) {
				o.WindSpeedMS = known(6)
				o.WindGustMS = known(10)
			}),
			wantMin:     60,
			wantMax:     94,
			wantVerdict: models.VerdictGo,
		},
		{
			name:     "strong wind walk needs caution",
			activity: models.ActivityWalk,
			obs: observation(func(o *models.WeatherObservation) {
				o.WindSpeedMS = known(11)
				o.WindGustMS = known(17)
			}),
			wantMin:     50,
			wantMax:     74,
			wantVerdict: models.VerdictCaution,
		},
		{
			name:     "storm gust walk is blocked",
			activity: models.ActivityWalk,
			obs: observation(func(o *models.WeatherObservation) {
				o.WindSpeedMS = known(11)
				o.WindGustMS = known(22)
			}),
			wantMin:     50,
			wantMax:     74,
			wantVerdict: models.VerdictNoGo,
		},
		{
			name:     "sup at wave tolerance edge",
			activity: models.ActivitySup,
			obs: observation(func(o *models.WeatherObservation) {
				o.WindSpeedMS = known(2)
				o.WindGustMS = known(4)
				o.WaveHeightM = known(0.4)
				o.WaterTempC = known(20)
			}),
			wantMin:     85,
			wantMax:     100,
			wantVerdict: models.VerdictGo,
		},
		{
			name:     "sup just above wave warning threshold",
			activity: models.ActivitySup,
			obs: observation(func(o *models.WeatherObservation) {
				o.WaveHeightM = known(0.5)
			}),
			wantMin:     55,
			wantMax:     80,
			wantVerdict: models.VerdictCaution,
		},
		{
			name:     "sup at wave danger threshold",
			activity: models.ActivitySup,
			obs: observation(func(o *models.WeatherObservation) {
				o.WaveHeightM = known(1.0)
			}),
			wantMin:     0,
			wantMax:     45,
			wantVerdict: models.VerdictNoGo,
		},
		{
			name:     "hike with heavy rain",
			activity: models.ActivityHike,
			obs: observation(func(o *models.WeatherObservation) {
				o.PrecipMM = known(3)
			}),
			wantMin:     40,
			wantMax:     80,
			wantVerdict: models.VerdictCaution,
		},
		{
			name:     "run with unhealthy air",
			activity: models.ActivityRun,
			obs: observation(func(o *models.WeatherObservation) {
				o.AQIEuropean = known(75)
			}),
			wantMin:     0,
			wantMax:     80,
			wantVerdict: models.VerdictCaution,
		},
		{
			name:     "walk in thunderstorm",
			activity: models.ActivityWalk,
			obs: observation(func(o *models.WeatherObservation) {
				o.WeatherCode = known(95)
			}),
			wantMin:     0,
			wantMax:     100,
			wantVerdict: models.VerdictNoGo,
		},
		{
			name:     "sup in freezing water",
			activity: models.ActivitySup,
			obs: observation(func(o *models.WeatherObservation) {
				o.WaterTempC = known(6)
			}),
			wantMin:     0,
			wantMax:     60,
			wantVerdict: models.VerdictNoGo,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assessment := evaluate(t, tc.activity, tc.obs)

			if int(assessment.Score) < tc.wantMin || int(assessment.Score) > tc.wantMax {
				t.Fatalf("score %d outside expected range [%d, %d]", assessment.Score, tc.wantMin, tc.wantMax)
			}
			if assessment.Verdict != tc.wantVerdict {
				t.Fatalf("verdict = %s, want %s (score %d, warnings %+v)",
					assessment.Verdict, tc.wantVerdict, assessment.Score, assessment.Warnings)
			}
			if assessment.AlgorithmVersion != scoring.AlgorithmVersion {
				t.Fatalf("assessment must carry algorithm version %q", scoring.AlgorithmVersion)
			}
		})
	}
}

// A high comfort score must not hide a safety problem.
func TestHighScoreDoesNotGuaranteeSafety(t *testing.T) {
	t.Parallel()

	assessment := evaluate(t, models.ActivityWalk, observation(func(o *models.WeatherObservation) {
		o.WeatherCode = known(95)
	}))

	if int(assessment.Score) < 90 {
		t.Fatalf("expected high comfort score for this case, got %d", assessment.Score)
	}
	if assessment.Verdict != models.VerdictNoGo {
		t.Fatalf("thunderstorm must force no_go regardless of comfort, got %s", assessment.Verdict)
	}
}

// AQI is an independent source: its outage must not break the forecast.
func TestAQIOutageKeepsForecastUsable(t *testing.T) {
	t.Parallel()

	assessment := evaluate(t, models.ActivityWalk, observation(func(o *models.WeatherObservation) {
		o.AQIEuropean = unknown()
		o.PM25 = unknown()
	}))

	if assessment.Verdict == models.VerdictNoGo {
		t.Fatalf("AQI outage must not block a walk, got %s", assessment.Verdict)
	}
	if assessment.Confidence == models.ConfidenceHigh {
		t.Fatal("confidence must drop when AQI is unavailable")
	}
	if !containsSubstring(assessment.Limitations, "AQI") {
		t.Fatalf("AQI outage must be reported in limitations, got %+v", assessment.Limitations)
	}
}

func TestDemoDataIsNeverSilentlyTrusted(t *testing.T) {
	t.Parallel()

	sources := []models.SourceStatus{
		{Source: models.SourceOpenMeteoWeather, Available: true, Demo: true},
	}
	assessment, err := scoring.Evaluate(models.ActivityWalk, "place-1", observation(), sources, true)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	if assessment.Verdict == models.VerdictGo {
		t.Fatal("demo data must not produce a go verdict")
	}
	if !hasWarning(assessment, "demo_data") {
		t.Fatalf("expected demo_data warning, got %+v", assessment.Warnings)
	}
	if !assessment.Demo {
		t.Fatal("assessment must be marked as demo")
	}
}

func TestExplicitMissingValues(t *testing.T) {
	t.Parallel()

	if unknown().Known {
		t.Fatal("zero-value measure must be unknown, not zero")
	}
	if !models.NewMeasure(0).Known {
		t.Fatal("explicit zero must be known")
	}
	if models.NewMeasure(0).Known == unknown().Known {
		t.Fatal("explicit zero and unknown must be distinguishable")
	}
}

func hasWarning(assessment models.ConditionAssessment, code string) bool {
	_, ok := findWarning(assessment, code)

	return ok
}

func findWarning(assessment models.ConditionAssessment, code string) (models.Warning, bool) {
	for _, warning := range assessment.Warnings {
		if warning.Code == code {
			return warning, true
		}
	}

	return models.Warning{}, false
}

func containsSubstring(items []string, substring string) bool {
	for _, item := range items {
		if strings.Contains(item, substring) {
			return true
		}
	}

	return false
}
