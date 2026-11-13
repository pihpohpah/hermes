package presenter

import (
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// ObservationDTO exposes every measurement explicitly. Unknown values are
// JSON null, never zero.
type ObservationDTO struct {
	AirTempC        models.Measure `json:"air_temp_c"`
	ApparentTempC   models.Measure `json:"apparent_temp_c"`
	PrecipitationMM models.Measure `json:"precipitation_mm"`
	WindSpeedMS     models.Measure `json:"wind_speed_ms"`
	WindGustMS      models.Measure `json:"wind_gust_ms"`
	WindDirection   models.Measure `json:"wind_direction_deg"`
	HumidityPct     models.Measure `json:"relative_humidity_pct"`
	CloudCoverPct   models.Measure `json:"cloud_cover_pct"`
	VisibilityM     models.Measure `json:"visibility_m"`
	WeatherCode     models.Measure `json:"weather_code"`
	WaveHeightM     models.Measure `json:"wave_height_m"`
	WavePeriodS     models.Measure `json:"wave_period_s"`
	WaveDirection   models.Measure `json:"wave_direction_deg"`
	WaterTempC      models.Measure `json:"water_temp_c"`
	SeaLevelHeightM models.Measure `json:"sea_level_height_m"`
	AirQualityIndex models.Measure `json:"air_quality_index"`
	PM25            models.Measure `json:"pm2_5"`
}

type SourceDTO struct {
	Source    models.SourceID `json:"source"`
	Available bool            `json:"available"`
	Reason    string          `json:"reason,omitempty"`
	Demo      bool            `json:"demo"`
}

// CoverageDTO reports how far each source actually reaches, so a client can see
// that AQI ends earlier than the weather forecast.
type CoverageDTO struct {
	Source       string          `json:"source"`
	Available    bool            `json:"available"`
	Demo         bool            `json:"demo"`
	Reason       string          `json:"reason,omitempty"`
	From         string          `json:"from,omitempty"`
	To           string          `json:"to,omitempty"`
	Hours        int             `json:"hours"`
	CoveredHours int             `json:"covered_hours"`
	Fields       []models.Factor `json:"fields"`
	FetchedAt    string          `json:"fetched_at,omitempty"`
}

// ModelGeometryDTO tells the client which model cell produced the forecast and
// how far the place altitude is from that cell.
type ModelGeometryDTO struct {
	ModelLatitude     models.Measure `json:"model_latitude"`
	ModelLongitude    models.Measure `json:"model_longitude"`
	ModelElevationM   models.Measure `json:"model_elevation_m"`
	PlaceElevationM   models.Measure `json:"place_elevation_m"`
	ElevationSource   string         `json:"elevation_source,omitempty"`
	Terrain           string         `json:"terrain,omitempty"`
	LapseCorrectionC  models.Measure `json:"lapse_correction_c"`
	CoarseModelNotice string         `json:"coarse_model_notice,omitempty"`
}

type ContributionDTO struct {
	Factor  models.Factor  `json:"factor"`
	Value   models.Measure `json:"value"`
	Penalty float64        `json:"penalty"`
	Note    string         `json:"note,omitempty"`
}

type WarningDTO struct {
	Code    string           `json:"code"`
	Level   models.RiskLevel `json:"level"`
	Factor  models.Factor    `json:"factor"`
	Value   models.Measure   `json:"value"`
	Message string           `json:"message"`
}

// ConditionAssessmentDTO is the scored view of one hour for one activity.
type ConditionAssessmentDTO struct {
	PlaceID          string            `json:"place_id"`
	Activity         models.Activity   `json:"activity"`
	ForecastTime     string            `json:"forecast_time"`
	LocalDate        string            `json:"local_date"`
	Score            models.Score      `json:"comfort_score"`
	Verdict          models.Verdict    `json:"verdict"`
	Confidence       models.Confidence `json:"confidence"`
	Summary          string            `json:"summary"`
	Contributions    []ContributionDTO `json:"contributions"`
	Warnings         []WarningDTO      `json:"warnings"`
	Limitations      []string          `json:"limitations"`
	Observation      ObservationDTO    `json:"observation"`
	Geometry         *ModelGeometryDTO `json:"geometry,omitempty"`
	Sources          []SourceDTO       `json:"sources"`
	Coverage         []CoverageDTO     `json:"coverage"`
	Demo             bool              `json:"demo"`
	AlgorithmVersion string            `json:"algorithm_version"`
	SafetyNotice     string            `json:"safety_notice"`
}

const safetyNotice = "Комфортный балл не гарантирует безопасность: проверяйте предупреждения, " +
	"прогноз и фактическую обстановку на месте."

func ObservationToHTTP(in models.WeatherObservation) ObservationDTO {
	return ObservationDTO{
		AirTempC:        in.AirTempC,
		ApparentTempC:   in.ApparentTempC,
		PrecipitationMM: in.PrecipMM,
		WindSpeedMS:     in.WindSpeedMS,
		WindGustMS:      in.WindGustMS,
		WindDirection:   in.WindDirection,
		HumidityPct:     in.HumidityPct,
		CloudCoverPct:   in.CloudCoverPct,
		VisibilityM:     in.VisibilityM,
		WeatherCode:     in.WeatherCode,
		WaveHeightM:     in.WaveHeightM,
		WavePeriodS:     in.WavePeriodS,
		WaveDirection:   in.WaveDirection,
		WaterTempC:      in.WaterTempC,
		SeaLevelHeightM: in.SeaLevelHeightM,
		AirQualityIndex: in.AQIEuropean,
		PM25:            in.PM25,
	}
}

func ConditionAssessmentToHTTP(in models.ConditionAssessment) ConditionAssessmentDTO {
	contributions := make([]ContributionDTO, 0, len(in.Contributions))
	for _, item := range in.Contributions {
		contributions = append(contributions, ContributionDTO{
			Factor:  item.Factor,
			Value:   item.Value,
			Penalty: item.Penalty,
			Note:    item.Note,
		})
	}

	warnings := make([]WarningDTO, 0, len(in.Warnings))
	for _, item := range in.Warnings {
		warnings = append(warnings, WarningDTO{
			Code:    item.Code,
			Level:   item.Level,
			Factor:  item.Factor,
			Value:   item.Value,
			Message: item.Message,
		})
	}

	sources := make([]SourceDTO, 0, len(in.Sources))
	for _, item := range in.Sources {
		sources = append(sources, SourceDTO{
			Source:    item.Source,
			Available: item.Available,
			Reason:    item.Reason,
			Demo:      item.Demo,
		})
	}

	limitations := in.Limitations
	if limitations == nil {
		limitations = []string{}
	}

	return ConditionAssessmentDTO{
		PlaceID:          in.PlaceID,
		Activity:         in.Activity,
		ForecastTime:     in.ForecastTime.UTC().Format(time.RFC3339),
		LocalDate:        in.LocalDate,
		Score:            in.Score,
		Verdict:          in.Verdict,
		Confidence:       in.Confidence,
		Summary:          in.Summary,
		Contributions:    contributions,
		Warnings:         warnings,
		Limitations:      limitations,
		Observation:      ObservationToHTTP(in.Observation),
		Geometry:         GeometryToHTTP(in.Geometry),
		Sources:          sources,
		Coverage:         coverageToHTTP(in.Coverage),
		Demo:             in.Demo,
		AlgorithmVersion: in.AlgorithmVersion,
		SafetyNotice:     safetyNotice,
	}
}

func ConditionAssessmentsToHTTP(in []models.ConditionAssessment) []ConditionAssessmentDTO {
	out := make([]ConditionAssessmentDTO, len(in))
	for i := range in {
		out[i] = ConditionAssessmentToHTTP(in[i])
	}

	return out
}

type Place struct {
	ID          string                  `json:"id"`
	Slug        string                  `json:"slug,omitempty"`
	Name        string                  `json:"name"`
	Subtitle    string                  `json:"subtitle,omitempty"`
	Short       string                  `json:"short,omitempty"`
	Description string                  `json:"description"`
	Type        models.PlaceType        `json:"type"`
	Layer       models.LayerCode        `json:"layer"`
	Category    string                  `json:"category,omitempty"`
	District    string                  `json:"district,omitempty"`
	Address     string                  `json:"address,omitempty"`
	Lat         float64                 `json:"lat"`
	Lon         float64                 `json:"lon"`
	Hours       string                  `json:"hours,omitempty"`
	PriceFree   *bool                   `json:"price_free,omitempty"`
	PriceLabel  string                  `json:"price_label,omitempty"`
	Duration    string                  `json:"duration,omitempty"`
	Season      string                  `json:"season,omitempty"`
	Access      []string                `json:"access"`
	HowToGet    string                  `json:"how_to_get,omitempty"`
	Facts       []string                `json:"facts"`
	Tags        []string                `json:"tags"`
	Photos      []models.Photo          `json:"photos"`
	Beach       string                  `json:"beach,omitempty"`
	SwimSeason  string                  `json:"swim_season,omitempty"`
	DistanceM   *float64                `json:"distance_m,omitempty"`
	Elevation   *int                    `json:"elevation,omitempty"`
	Difficulty  *models.Difficulty      `json:"difficulty,omitempty"`
	OSMID       *int64                  `json:"osm_id,omitempty"`
	LatestScore *ConditionAssessmentDTO `json:"latest_score,omitempty"`
	CreatedAt   string                  `json:"created_at"`
}

func PlaceToHTTP(in models.Place) Place {
	var latest *ConditionAssessmentDTO
	if in.LatestScore != nil {
		mapped := ConditionAssessmentToHTTP(*in.LatestScore)
		latest = &mapped
	}

	access := make([]string, 0, len(in.Access))
	for _, mode := range in.Access {
		access = append(access, string(mode))
	}

	out := Place{
		ID:          in.ID,
		Name:        in.Name,
		Subtitle:    in.Subtitle,
		Short:       in.Short,
		Description: in.Description,
		Type:        in.Type,
		Layer:       in.Layer,
		District:    in.District,
		Address:     in.Address,
		Lat:         in.Location.Lat,
		Lon:         in.Location.Lon,
		Hours:       in.Hours,
		PriceFree:   in.PriceFree,
		PriceLabel:  in.PriceLabel,
		Duration:    in.Duration,
		Season:      in.Season,
		Access:      access,
		HowToGet:    in.HowToGet,
		Facts:       in.Facts,
		Tags:        in.Tags,
		Photos:      in.Photos,
		SwimSeason:  in.SwimSeason,
		DistanceM:   in.DistanceM,
		Elevation:   in.Elevation,
		Difficulty:  in.Difficulty,
		OSMID:       in.OSMID,
		LatestScore: latest,
		CreatedAt:   in.CreatedAt.UTC().Format(time.RFC3339),
	}
	if in.Slug != nil {
		out.Slug = *in.Slug
	}
	if in.Category != nil {
		out.Category = string(*in.Category)
	}
	if in.Beach != nil {
		out.Beach = string(*in.Beach)
	}

	return out
}

func PlacesToHTTP(in []models.Place) []Place {
	out := make([]Place, len(in))
	for i := range in {
		out[i] = PlaceToHTTP(in[i])
	}

	return out
}

const coarseModelNotice = "Модель Open-Meteo — сетка около 11 км: высота и температура на " +
	"вершине могут отличаться от прогноза в этой точке."

// GeometryToHTTP maps model geometry; a nil/zero geometry is omitted.
func GeometryToHTTP(in models.ModelGeometry) *ModelGeometryDTO {
	if !in.ModelLatitude.Known && !in.PlaceElevationM.Known && !in.ModelElevationM.Known {
		return nil
	}

	dto := &ModelGeometryDTO{
		ModelLatitude:    in.ModelLatitude,
		ModelLongitude:   in.ModelLongitude,
		ModelElevationM:  in.ModelElevationM,
		PlaceElevationM:  in.PlaceElevationM,
		ElevationSource:  in.ElevationSource,
		Terrain:          in.Terrain,
		LapseCorrectionC: in.LapseCorrectionC,
	}
	if in.Terrain == string(models.TerrainHilly) || in.Terrain == string(models.TerrainMountainous) {
		dto.CoarseModelNotice = coarseModelNotice
	}

	return dto
}

func coverageToHTTP(in []models.SourceCoverage) []CoverageDTO {
	out := make([]CoverageDTO, 0, len(in))
	for _, item := range in {
		fields := item.Fields
		if fields == nil {
			fields = []models.Factor{}
		}
		out = append(out, CoverageDTO{
			Source:       string(item.Kind),
			Available:    item.Available,
			Demo:         item.Demo,
			Reason:       item.Reason,
			From:         item.From,
			To:           item.To,
			Hours:        item.Hours,
			CoveredHours: item.Covered,
			Fields:       fields,
			FetchedAt:    item.FetchedAt,
		})
	}

	return out
}

// ForecastRefreshResult is the payload of POST /forecast/refresh.
// RefreshedAt is already an RFC3339 string: the use case formats it once so the
// API and the logs agree on the timestamp.
type ForecastRefreshResult struct {
	UpdatedSlots int    `json:"updated_slots"`
	RefreshedAt  string `json:"refreshed_at"`
}

// Trip is the HTTP representation of a planned outing.
type Trip struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	PlaceID   string `json:"place_id"`
	PlannedAt string `json:"planned_at"`
	Activity  string `json:"activity"`
	Status    string `json:"status"`
	Notes     string `json:"notes"`
	CreatedAt string `json:"created_at"`
}

func TripToHTTP(in models.Trip) Trip {
	return Trip{
		ID:        in.ID,
		UserID:    in.UserID,
		PlaceID:   in.PlaceID,
		PlannedAt: in.PlannedAt.UTC().Format(time.RFC3339),
		Activity:  string(in.Activity),
		Status:    string(in.Status),
		Notes:     in.Notes,
		CreatedAt: in.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func TripsToHTTP(in []models.Trip) []Trip {
	out := make([]Trip, len(in))
	for i := range in {
		out[i] = TripToHTTP(in[i])
	}

	return out
}

// Report is the HTTP representation of a crowd-sourced observation.
type Report struct {
	ID        string  `json:"id"`
	PlaceID   string  `json:"place_id"`
	UserID    *string `json:"user_id,omitempty"`
	Type      string  `json:"type"`
	Comment   string  `json:"comment"`
	CreatedAt string  `json:"created_at"`
}

func ReportToHTTP(in models.Report) Report {
	return Report{
		ID:        in.ID,
		PlaceID:   in.PlaceID,
		UserID:    in.UserID,
		Type:      string(in.Type),
		Comment:   in.Comment,
		CreatedAt: in.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func ReportsToHTTP(in []models.Report) []Report {
	out := make([]Report, len(in))
	for i := range in {
		out[i] = ReportToHTTP(in[i])
	}

	return out
}
