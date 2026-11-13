package scoring

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

const (
	baseScore = 100.0
	minScore  = 0
	maxScore  = 100

	goScoreThreshold      = 75
	cautionScoreThreshold = 50
	missingDataScoreCap   = 55

	severityInfo    = 1
	severityCaution = 2
	severityDanger  = 3

	penaltyPartCapacity = 8
	warningCapacity     = 6
	limitationCapacity  = 4
	percentScale        = 100
	aqiDangerFactor     = 1.5
)

// Evaluate scores one hourly observation for one activity. It returns an
// explainable assessment: a comfort score with per-factor contributions and a
// separate list of safety warnings.
func Evaluate(
	activity models.Activity,
	placeID string,
	obs models.WeatherObservation,
	sources []models.SourceStatus,
	demo bool,
) (models.ConditionAssessment, error) {
	profile, err := ProfileFor(activity)
	if err != nil {
		return models.ConditionAssessment{}, fmt.Errorf("evaluate activity `%s`: %w", activity, err)
	}

	penalty, contributions := comfortPenalty(profile, obs)

	scoreValue := int(math.Round(clamp(baseScore - penalty)))
	score, err := models.NewScore(scoreValue)
	if err != nil {
		return models.ConditionAssessment{}, fmt.Errorf("evaluate comfort score: %w", err)
	}

	warnings, limitations, criticalMissing := safetyWarnings(profile, obs, sources, demo)

	verdict := verdictFor(score, warnings, criticalMissing, demo)
	if criticalMissing {
		capped := min(int(score), missingDataScoreCap)
		score, err = models.NewScore(capped)
		if err != nil {
			return models.ConditionAssessment{}, fmt.Errorf("cap comfort score: %w", err)
		}
	}

	confidence := confidenceFor(profile, obs, criticalMissing, demo)

	return models.ConditionAssessment{
		PlaceID:          placeID,
		Activity:         activity,
		ForecastTime:     obs.Hour.UTC(),
		LocalDate:        localDate(obs.Hour),
		Observation:      obs,
		Score:            score,
		Verdict:          verdict,
		Confidence:       confidence,
		Limitations:      limitations,
		Summary:          summaryFor(profile.Activity, score, verdict, warnings, contributions),
		Contributions:    contributions,
		Warnings:         warnings,
		Sources:          sources,
		Demo:             demo,
		AlgorithmVersion: AlgorithmVersion,
	}, nil
}

type penaltyPart struct {
	factor  models.Factor
	value   models.Measure
	penalty float64
	note    string
}

func comfortPenalty(profile Profile, obs models.WeatherObservation) (float64, []models.Contribution) {
	parts := make([]penaltyPart, 0, penaltyPartCapacity)
	model := profile.Model

	if v, ok := known(obs.WindSpeedMS); ok {
		parts = append(parts, penaltyPart{
			factor:  models.FactorWindSpeed,
			value:   obs.WindSpeedMS,
			penalty: model.WindSpeedPerMS * v,
			note:    "скорость ветра",
		})
	}
	if v, ok := known(obs.WindGustMS); ok && v > profile.WindGustToleranceMS {
		parts = append(parts, penaltyPart{
			factor:  models.FactorWindGust,
			value:   obs.WindGustMS,
			penalty: model.WindGustExcessPerMS * (v - profile.WindGustToleranceMS),
			note:    fmt.Sprintf("порывы выше допуска %.0f м/с", profile.WindGustToleranceMS),
		})
	}
	if profile.RequiresMarine {
		if v, ok := known(obs.WaveHeightM); ok && v > profile.WaveToleranceM {
			parts = append(parts, penaltyPart{
				factor:  models.FactorWaveHeight,
				value:   obs.WaveHeightM,
				penalty: model.WaveHeightPerM * (v - profile.WaveToleranceM),
				note:    fmt.Sprintf("волна выше допуска %.1f м", profile.WaveToleranceM),
			})
		}
	}
	if v, ok := known(obs.PrecipMM); ok {
		parts = append(parts, penaltyPart{
			factor:  models.FactorPrecipitation,
			value:   obs.PrecipMM,
			penalty: model.PrecipPerMM * v,
			note:    "осадки за час",
		})
	}
	if v, ok := known(obs.AQIEuropean); ok && v > profile.AIRefLevel {
		parts = append(parts, penaltyPart{
			factor:  models.FactorAirQuality,
			value:   obs.AQIEuropean,
			penalty: model.AQIPerPoint * (v - profile.AIRefLevel),
			note:    fmt.Sprintf("AQI выше фона %.0f", profile.AIRefLevel),
		})
	}
	if profile.RequiresMarine {
		if v, ok := known(obs.WaterTempC); ok && v < profile.ColdWaterTempC {
			parts = append(parts, penaltyPart{
				factor:  models.FactorWaterTemperature,
				value:   obs.WaterTempC,
				penalty: model.ColdWaterPerDegC * (profile.ColdWaterTempC - v),
				note:    fmt.Sprintf("вода холоднее %.0f °C", profile.ColdWaterTempC),
			})
		}
	}
	if v, ok := known(obs.AirTempC); ok {
		if v > profile.ComfortAirTempHighC {
			parts = append(parts, penaltyPart{
				factor:  models.FactorAirTemperature,
				value:   obs.AirTempC,
				penalty: model.AirComfortPerDegC * (v - profile.ComfortAirTempHighC),
				note:    fmt.Sprintf("жарче комфортных %.0f °C", profile.ComfortAirTempHighC),
			})
		}
		if v < profile.ComfortAirTempLowC {
			parts = append(parts, penaltyPart{
				factor:  models.FactorAirTemperature,
				value:   obs.AirTempC,
				penalty: model.AirComfortPerDegC * (profile.ComfortAirTempLowC - v),
				note:    fmt.Sprintf("холоднее комфортных %.0f °C", profile.ComfortAirTempLowC),
			})
		}
	}

	contributions := make([]models.Contribution, 0, len(parts))
	total := 0.0
	for _, part := range parts {
		rounded := round2(part.penalty)
		total += rounded
		contributions = append(contributions, models.Contribution{
			Factor:  part.factor,
			Value:   part.value,
			Penalty: rounded,
			Note:    part.note,
		})
	}
	sort.SliceStable(contributions, func(i, j int) bool {
		return contributions[i].Penalty > contributions[j].Penalty
	})

	return round2(total), contributions
}

type warningCollector struct {
	warnings []models.Warning
}

func (c *warningCollector) add(
	code string,
	level models.RiskLevel,
	factor models.Factor,
	value models.Measure,
	message string,
) {
	c.warnings = append(c.warnings, models.Warning{
		Code:    code,
		Level:   level,
		Factor:  factor,
		Value:   value,
		Message: message,
	})
}

func safetyWarnings(
	profile Profile,
	obs models.WeatherObservation,
	sources []models.SourceStatus,
	demo bool,
) ([]models.Warning, []string, bool) {
	collector := &warningCollector{warnings: make([]models.Warning, 0, warningCapacity)}
	limitations := make([]string, 0, limitationCapacity)

	criticalMissing := missingCriticalWarnings(profile, obs, collector, &limitations)
	warnWind(profile, obs, collector)
	warnMarine(profile, obs, collector)
	warnPrecipitation(profile, obs, collector)
	warnAirQuality(profile, obs, collector)
	warnTemperature(profile, obs, collector)
	warnVisibilityAndPhenomena(profile, obs, collector)
	limitations = append(limitations, sourceLimitations(sources)...)

	if !obs.AQIEuropean.Known {
		limitations = append(limitations, "AQI недоступен — оценка не учитывает качество воздуха")
	}
	if demo {
		limitations = append(limitations, "демонстрационные данные: погода сгенерирована, а не получена из сети")
		collector.add("demo_data", models.RiskCaution, models.FactorWindSpeed, models.MissingMeasure(),
			"демо-режим: не используйте оценку для реального выхода")
	}

	return collector.warnings, limitations, criticalMissing
}

func missingCriticalWarnings(
	profile Profile,
	obs models.WeatherObservation,
	collector *warningCollector,
	limitations *[]string,
) bool {
	missing := false
	for _, critical := range profile.CriticalFactors() {
		if obs.Field(critical.Factor).Known {
			continue
		}
		missing = true
		*limitations = append(
			*limitations,
			critical.Message+" — оценка ограничена, рекомендация не может быть «зелёной»",
		)
		collector.add(
			"missing_"+string(critical.Factor),
			models.RiskCaution,
			critical.Factor,
			models.MissingMeasure(),
			critical.Message+" — это не значит, что условия безопасны",
		)
	}

	return missing
}

func warnWind(profile Profile, obs models.WeatherObservation, collector *warningCollector) {
	if v, ok := known(obs.WindGustMS); ok {
		switch {
		case v >= profile.GustDangerMS:
			collector.add("wind_gust_danger", models.RiskDanger, models.FactorWindGust, obs.WindGustMS,
				fmt.Sprintf("порывы %.1f м/с — опасны для этой активности", v))
		case v >= profile.GustWarningMS:
			collector.add("wind_gust", models.RiskCaution, models.FactorWindGust, obs.WindGustMS,
				fmt.Sprintf("порывы %.1f м/с — работайте с запасом", v))
		}
	}
	if v, ok := known(obs.WindSpeedMS); ok && v >= profile.WindWarningMS {
		collector.add("wind", models.RiskCaution, models.FactorWindSpeed, obs.WindSpeedMS,
			fmt.Sprintf("устойчивый ветер %.1f м/с", v))
	}
}

func warnMarine(profile Profile, obs models.WeatherObservation, collector *warningCollector) {
	if !profile.RequiresMarine {
		return
	}

	if v, ok := known(obs.WaveHeightM); ok {
		switch {
		case v >= profile.WaveDangerM:
			collector.add("wave_danger", models.RiskDanger, models.FactorWaveHeight, obs.WaveHeightM,
				fmt.Sprintf("волна %.2f м — выход на воду небезопасен", v))
		case v >= profile.WaveWarningM:
			collector.add("wave", models.RiskCaution, models.FactorWaveHeight, obs.WaveHeightM,
				fmt.Sprintf("волна %.2f м — подходит только опытным", v))
		}
	}
	if v, ok := known(obs.WaterTempC); ok && v < profile.WaterTempAwarenessC {
		collector.add("cold_water", models.RiskCaution, models.FactorWaterTemperature, obs.WaterTempC,
			fmt.Sprintf("вода %.1f °C — риск холодового шока", v))
	}
}

func warnPrecipitation(profile Profile, obs models.WeatherObservation, collector *warningCollector) {
	v, ok := known(obs.PrecipMM)
	if !ok {
		return
	}

	switch {
	case v >= profile.HeavyRainMM:
		collector.add("heavy_precipitation", models.RiskCaution, models.FactorPrecipitation, obs.PrecipMM,
			fmt.Sprintf("сильные осадки %.1f мм/ч", v))
	case v >= profile.SlipperyPrecipMM:
		collector.add("precipitation", models.RiskInfo, models.FactorPrecipitation, obs.PrecipMM,
			fmt.Sprintf("небольшие осадки %.1f мм/ч — скользко", v))
	}
}

func warnAirQuality(profile Profile, obs models.WeatherObservation, collector *warningCollector) {
	v, ok := known(obs.AQIEuropean)
	if !ok {
		return
	}

	switch {
	case v >= profile.AQAwareness*aqiDangerFactor:
		collector.add("air_quality_danger", models.RiskDanger, models.FactorAirQuality, obs.AQIEuropean,
			fmt.Sprintf("AQI %.0f — тренировки на улице противопоказаны", v))
	case v >= profile.AQAwareness:
		collector.add("air_quality", models.RiskCaution, models.FactorAirQuality, obs.AQIEuropean,
			fmt.Sprintf("AQI %.0f — чувствительным людям стоит сократить нагрузку", v))
	}
}

func warnTemperature(profile Profile, obs models.WeatherObservation, collector *warningCollector) {
	v, ok := known(obs.AirTempC)
	if !ok {
		return
	}

	if v <= profile.AirTempAwarenessLowC {
		collector.add("cold_air", models.RiskCaution, models.FactorAirTemperature, obs.AirTempC,
			fmt.Sprintf("воздух %.1f °C — риск переохлаждения", v))
	}
	if v >= profile.AirTempAwarenessHiC {
		collector.add("hot_air", models.RiskCaution, models.FactorAirTemperature, obs.AirTempC,
			fmt.Sprintf("воздух %.1f °C — риск перегрева", v))
	}
}

func warnVisibilityAndPhenomena(profile Profile, obs models.WeatherObservation, collector *warningCollector) {
	if v, ok := known(obs.VisibilityM); ok && v < profile.PoorVisibilityM {
		collector.add("poor_visibility", models.RiskCaution, models.FactorVisibility, obs.VisibilityM,
			fmt.Sprintf("видимость %.0f м — легко потерять ориентиры", v))
	}

	v, ok := known(obs.WeatherCode)
	if !ok {
		return
	}
	if isThunderstorm(v) {
		collector.add("thunderstorm", models.RiskDanger, models.FactorWeatherCode, obs.WeatherCode,
			"гроза — выход на маршрут и воду запрещён")
	}
	if isFreezingRain(v) {
		collector.add("freezing_rain", models.RiskDanger, models.FactorWeatherCode, obs.WeatherCode,
			"ледяной дождь — высокий риск травм")
	}
}

func sourceLimitations(sources []models.SourceStatus) []string {
	limitations := make([]string, 0, len(sources))
	for _, source := range sources {
		if !source.Available && source.Reason != "" && !source.Demo {
			limitations = append(limitations, source.Reason)
		}
	}

	return limitations
}

func verdictFor(
	score models.Score,
	warnings []models.Warning,
	criticalMissing bool,
	demo bool,
) models.Verdict {
	severity := 0
	for _, warning := range warnings {
		severity = max(severity, levelSeverity(warning.Level))
	}

	verdict := models.VerdictGo
	switch {
	case int(score) < cautionScoreThreshold || severity >= severityDanger:
		verdict = models.VerdictNoGo
	case int(score) < goScoreThreshold || severity >= severityCaution:
		verdict = models.VerdictCaution
	}

	if criticalMissing || demo {
		if verdict == models.VerdictGo {
			verdict = models.VerdictCaution
		}
	}

	return verdict
}

func confidenceFor(
	profile Profile,
	obs models.WeatherObservation,
	criticalMissing bool,
	demo bool,
) models.Confidence {
	if criticalMissing {
		return models.ConfidenceLow
	}

	missingAdvisory := 0
	for _, advisory := range profile.AdvisoryFactors() {
		if !obs.Field(advisory.Factor).Known {
			missingAdvisory++
		}
	}
	if demo {
		return models.ConfidenceMedium
	}
	if missingAdvisory > 0 {
		return models.ConfidenceMedium
	}

	return models.ConfidenceHigh
}

func summaryFor(
	activity models.Activity,
	score models.Score,
	verdict models.Verdict,
	warnings []models.Warning,
	contributions []models.Contribution,
) string {
	activityTitle := activityTitle(activity)
	drive := ""
	if len(contributions) > 0 {
		drive = ", сильнее всего мешает: " + contributionTitle(contributions[0])
	}
	if len(warnings) > 0 {
		worst := worstWarning(warnings)
		drive += ", предупреждение: " + worst.Message
	}

	switch verdict {
	case models.VerdictGo:
		return fmt.Sprintf("%s: комфорт %d/100, можно планировать%s", activityTitle, score, drive)
	case models.VerdictCaution:
		return fmt.Sprintf("%s: комфорт %d/100, нужна осторожность%s", activityTitle, score, drive)
	case models.VerdictNoGo:
		return fmt.Sprintf("%s: комфорт %d/100, выходить не рекомендуется%s", activityTitle, score, drive)
	default:
		return fmt.Sprintf("%s: комфорт %d/100%s", activityTitle, score, drive)
	}
}

func contributionTitle(c models.Contribution) string {
	titles := map[models.Factor]string{
		models.FactorWindSpeed:           "ветер",
		models.FactorWindGust:            "порывы",
		models.FactorWaveHeight:          "волна",
		models.FactorPrecipitation:       "осадки",
		models.FactorAirQuality:          "качество воздуха",
		models.FactorWaterTemperature:    "холодная вода",
		models.FactorAirTemperature:      "температура воздуха",
		models.FactorApparentTemperature: "ощущаемая температура",
		models.FactorVisibility:          "видимость",
		models.FactorWeatherCode:         "погодное явление",
	}
	if title, ok := titles[c.Factor]; ok {
		return title
	}

	return string(c.Factor)
}

func activityTitle(activity models.Activity) string {
	switch activity {
	case models.ActivityWalk:
		return "Прогулка"
	case models.ActivityRun:
		return "Пробежка"
	case models.ActivityHike:
		return "Поход"
	case models.ActivitySup:
		return "SUP"
	default:
		return string(activity)
	}
}

func worstWarning(warnings []models.Warning) models.Warning {
	worst := warnings[0]
	for _, warning := range warnings[1:] {
		if levelSeverity(warning.Level) > levelSeverity(worst.Level) {
			worst = warning
		}
	}

	return worst
}

func levelSeverity(level models.RiskLevel) int {
	switch level {
	case models.RiskDanger:
		return severityDanger
	case models.RiskCaution:
		return severityCaution
	case models.RiskInfo:
		return severityInfo
	default:
		return severityInfo
	}
}

func isThunderstorm(code float64) bool {
	return code == 95 || code == 96 || code == 99
}

func isFreezingRain(code float64) bool {
	return code == 66 || code == 67
}

func known(m models.Measure) (float64, bool) {
	return m.Value, m.Known
}

func localDate(instant time.Time) string {
	loc, err := models.PrimoryeLocation()
	if err != nil {
		return instant.UTC().Format("2006-01-02")
	}

	return instant.In(loc).Format("2006-01-02")
}

func clamp(value float64) float64 {
	return math.Max(math.Min(value, maxScore), minScore)
}

func round2(value float64) float64 {
	return math.Round(value*percentScale) / percentScale
}
