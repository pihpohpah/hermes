package scoring

import (
	"github.com/pihpohpah/hermes/internal/domain/models"
)

// AlgorithmVersion must be bumped whenever weights, thresholds or warning rules
// change, so stored assessments stay explainable.
const AlgorithmVersion = "heuristic-2.1.0"

type penaltyModel struct {
	WindSpeedPerMS      float64
	WindGustExcessPerMS float64
	WaveHeightPerM      float64
	PrecipPerMM         float64
	AQIPerPoint         float64
	ColdWaterPerDegC    float64
	AirComfortPerDegC   float64
}

type comfortModel struct {
	Model penaltyModel

	WindGustToleranceMS  float64
	WaveToleranceM       float64
	AIRefLevel           float64
	AirTempLowC          float64
	AirTempHighC         float64
	ComfortAirTempLowC   float64
	ComfortAirTempHighC  float64
	ColdWaterTempC       float64
	HeavyRainMM          float64
	PoorVisibilityM      float64
	SlipperyPrecipMM     float64
	WindWarningMS        float64
	GustWarningMS        float64
	WaveWarningM         float64
	WaveDangerM          float64
	GustDangerMS         float64
	AQAwareness          float64
	AirTempAwarenessLowC float64
	AirTempAwarenessHiC  float64
	WaterTempAwarenessC  float64
}

// Profile is the complete scoring configuration of one activity.
type Profile struct {
	Activity models.Activity
	comfortModel
	RequiresMarine bool
	RequiresAir    bool
}

//nolint:gochecknoglobals // scoring policy table, read-only by construction
var profiles = map[models.Activity]Profile{
	models.ActivityWalk: {
		Activity: models.ActivityWalk,
		comfortModel: comfortModel{
			Model: penaltyModel{
				WindSpeedPerMS:      2.2,
				WindGustExcessPerMS: 0.9,
				PrecipPerMM:         9.0,
				AQIPerPoint:         0.25,
				AirComfortPerDegC:   1.2,
			},
			WindGustToleranceMS:  8,
			AIRefLevel:           50,
			AirTempLowC:          -15,
			AirTempHighC:         33,
			ComfortAirTempLowC:   5,
			ComfortAirTempHighC:  24,
			HeavyRainMM:          2.0,
			PoorVisibilityM:      1000,
			SlipperyPrecipMM:     0.5,
			WindWarningMS:        10,
			GustWarningMS:        15,
			GustDangerMS:         20,
			AQAwareness:          100,
			AirTempAwarenessLowC: -10,
			AirTempAwarenessHiC:  29,
		},
	},
	models.ActivityRun: {
		Activity: models.ActivityRun,
		comfortModel: comfortModel{
			Model: penaltyModel{
				WindSpeedPerMS:      1.4,
				WindGustExcessPerMS: 0.7,
				PrecipPerMM:         11.0,
				AQIPerPoint:         0.45,
				AirComfortPerDegC:   1.0,
			},
			WindGustToleranceMS:  6,
			AIRefLevel:           30,
			AirTempLowC:          -12,
			AirTempHighC:         32,
			ComfortAirTempLowC:   2,
			ComfortAirTempHighC:  19,
			HeavyRainMM:          1.5,
			PoorVisibilityM:      800,
			SlipperyPrecipMM:     0.3,
			WindWarningMS:        9,
			GustWarningMS:        14,
			GustDangerMS:         19,
			AQAwareness:          60,
			AirTempAwarenessLowC: -8,
			AirTempAwarenessHiC:  27,
		},
	},
	models.ActivityHike: {
		Activity: models.ActivityHike,
		comfortModel: comfortModel{
			Model: penaltyModel{
				WindSpeedPerMS:      3.2,
				WindGustExcessPerMS: 1.3,
				PrecipPerMM:         12.0,
				AQIPerPoint:         0.30,
				AirComfortPerDegC:   1.4,
			},
			WindGustToleranceMS:  12,
			AIRefLevel:           60,
			AirTempLowC:          -20,
			AirTempHighC:         34,
			ComfortAirTempLowC:   0,
			ComfortAirTempHighC:  22,
			HeavyRainMM:          2.5,
			PoorVisibilityM:      500,
			SlipperyPrecipMM:     0.2,
			WindWarningMS:        13,
			GustWarningMS:        18,
			GustDangerMS:         24,
			AQAwareness:          100,
			AirTempAwarenessLowC: -12,
			AirTempAwarenessHiC:  30,
		},
	},
	models.ActivitySup: {
		Activity:       models.ActivitySup,
		RequiresMarine: true,
		RequiresAir:    true,
		comfortModel: comfortModel{
			Model: penaltyModel{
				WindSpeedPerMS:      6.5,
				WindGustExcessPerMS: 2.2,
				WaveHeightPerM:      90.0,
				PrecipPerMM:         9.0,
				AQIPerPoint:         0.12,
				ColdWaterPerDegC:    5.0,
				AirComfortPerDegC:   0.8,
			},
			WindGustToleranceMS:  6,
			WaveToleranceM:       0.4,
			AIRefLevel:           60,
			AirTempLowC:          6,
			AirTempHighC:         32,
			ComfortAirTempLowC:   17,
			ComfortAirTempHighC:  26,
			ColdWaterTempC:       16,
			HeavyRainMM:          1.5,
			PoorVisibilityM:      1500,
			SlipperyPrecipMM:     0.3,
			WindWarningMS:        6,
			GustWarningMS:        9,
			GustDangerMS:         14,
			WaveWarningM:         0.5,
			WaveDangerM:          1.0,
			AQAwareness:          100,
			AirTempAwarenessLowC: 10,
			AirTempAwarenessHiC:  28,
			WaterTempAwarenessC:  14,
		},
	},
}

// ProfileFor returns the scoring profile of an activity.
func ProfileFor(activity models.Activity) (Profile, error) {
	profile, ok := profiles[activity]
	if !ok {
		return Profile{}, models.Invalid("unexpected activity `" + string(activity) + "`")
	}

	return profile, nil
}

// CriticalFactor is an input without which the assessment cannot be trusted.
type CriticalFactor struct {
	Factor  models.Factor
	Message string
}

// CriticalFactors lists inputs that must be present for a trustworthy score.
func (p Profile) CriticalFactors() []CriticalFactor {
	critical := []CriticalFactor{
		{Factor: models.FactorWindSpeed, Message: "нет данных о скорости ветра"},
		{Factor: models.FactorAirTemperature, Message: "нет данных о температуре воздуха"},
	}
	if p.RequiresMarine {
		critical = append(critical,
			CriticalFactor{
				Factor:  models.FactorWaveHeight,
				Message: "высота волны неизвестна — это не «штиль», безопасность выхода на воду не подтверждена",
			},
			CriticalFactor{Factor: models.FactorWaterTemperature, Message: "температура воды неизвестна"},
		)
	}

	return critical
}

// AdvisoryFactors are optional inputs: their absence lowers confidence but
// never blocks the recommendation (e.g. an AQI outage).
func (p Profile) AdvisoryFactors() []CriticalFactor {
	return []CriticalFactor{{Factor: models.FactorAirQuality, Message: "качество воздуха недоступно"}}
}
