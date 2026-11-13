package models

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"time"
)

const float64BitSize = 64

// Measure is a weather value that may be unknown. Unknown values are never
// silently replaced with zero: they are serialized as null and must be
// handled explicitly by scorers, storage and clients.
type Measure struct {
	Value float64
	Known bool
}

func NewMeasure(value float64) Measure {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return Measure{}
	}

	return Measure{Value: value, Known: true}
}

func MissingMeasure() Measure {
	return Measure{}
}

func (m Measure) MarshalJSON() ([]byte, error) {
	if !m.Known {
		return []byte("null"), nil
	}

	return strconv.AppendFloat(nil, m.Value, 'f', -1, float64BitSize), nil
}

func (m *Measure) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*m = Measure{}

		return nil
	}

	var value float64
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*m = NewMeasure(value)

	return nil
}

func (m Measure) Float64Ptr() *float64 {
	if !m.Known {
		return nil
	}
	value := m.Value

	return &value
}

type SourceID string

const (
	SourceOpenMeteoWeather    SourceID = "open_meteo_weather"
	SourceOpenMeteoMarine     SourceID = "open_meteo_marine"
	SourceOpenMeteoAirQuality SourceID = "open_meteo_air_quality"
)

// SourceSource* are aliases used when a source slice is built.
const (
	SourceSourceWeather    = SourceOpenMeteoWeather
	SourceSourceMarine     = SourceOpenMeteoMarine
	SourceSourceAirQuality = SourceOpenMeteoAirQuality
)

// SourceIDForKind maps a kind to its upstream identifier.
func SourceIDForKind(kind SourceKind) SourceID {
	switch kind {
	case SourceKindWeather:
		return SourceOpenMeteoWeather
	case SourceKindMarine:
		return SourceOpenMeteoMarine
	case SourceKindAirQuality:
		return SourceOpenMeteoAirQuality
	default:
		return ""
	}
}

// SourceStatus tells the client which source produced the data, whether it was
// available and whether the source is a demo one.
type SourceStatus struct {
	Source    SourceID `json:"source"`
	Available bool     `json:"available"`
	Reason    string   `json:"reason,omitempty"`
	Demo      bool     `json:"demo,omitempty"`
}

// WeatherObservation is one hour of normalized weather data. Every numeric
// field is explicit about presence via Measure.
type WeatherObservation struct {
	Hour time.Time `json:"-"`

	AirTempC        Measure `json:"air_temp_c"`
	ApparentTempC   Measure `json:"apparent_temp_c"`
	PrecipMM        Measure `json:"precipitation_mm"`
	WindSpeedMS     Measure `json:"wind_speed_ms"`
	WindGustMS      Measure `json:"wind_gust_ms"`
	WindDirection   Measure `json:"wind_direction_deg"`
	HumidityPct     Measure `json:"relative_humidity_pct"`
	CloudCoverPct   Measure `json:"cloud_cover_pct"`
	VisibilityM     Measure `json:"visibility_m"`
	WeatherCode     Measure `json:"weather_code"`
	WaveHeightM     Measure `json:"wave_height_m"`
	WavePeriodS     Measure `json:"wave_period_s"`
	WaveDirection   Measure `json:"wave_direction_deg"`
	WaterTempC      Measure `json:"water_temp_c"`
	SeaLevelHeightM Measure `json:"sea_level_height_m"`
	AQIEuropean     Measure `json:"air_quality_index"`
	PM25            Measure `json:"pm2_5"`
}

func (o WeatherObservation) Field(factor Factor) Measure {
	switch factor {
	case FactorWindSpeed:
		return o.WindSpeedMS
	case FactorWindGust:
		return o.WindGustMS
	case FactorWaveHeight:
		return o.WaveHeightM
	case FactorPrecipitation:
		return o.PrecipMM
	case FactorAirQuality:
		return o.AQIEuropean
	case FactorWaterTemperature:
		return o.WaterTempC
	case FactorAirTemperature:
		return o.AirTempC
	case FactorApparentTemperature:
		return o.ApparentTempC
	case FactorVisibility:
		return o.VisibilityM
	case FactorWeatherCode:
		return o.WeatherCode
	default:
		return MissingMeasure()
	}
}

// ModelGeometry describes the grid cell a forecast actually came from. The
// public Open-Meteo model is coarse (~11 km), so a mountain point and a city
// point often share one cell; the client must be able to see that.
type ModelGeometry struct {
	ModelLatitude    Measure `json:"model_latitude"`
	ModelLongitude   Measure `json:"model_longitude"`
	ModelElevationM  Measure `json:"model_elevation_m"`
	PlaceElevationM  Measure `json:"place_elevation_m"`
	ElevationSource  string  `json:"elevation_source,omitempty"`
	Terrain          string  `json:"terrain,omitempty"`
	LapseCorrectionC Measure `json:"lapse_correction_c"`
}

func (s WeatherSeries) At(hour time.Time) (WeatherObservation, bool) {
	target := hour.UTC().Truncate(time.Hour)
	for _, obs := range s.Hours {
		if obs.Hour.Equal(target) {
			return obs, true
		}
	}

	return WeatherObservation{}, false
}

func (s WeatherSeries) IsDemo() bool {
	for _, source := range s.Sources {
		if source.Demo {
			return true
		}
	}

	return false
}

// SourceKind identifies one upstream data source. Sources are fetched, stored
// and reported independently: an air-quality outage must not affect marine,
// and a short AQI horizon must not shorten the weather forecast.
type SourceKind string

const (
	SourceKindWeather    SourceKind = "weather"
	SourceKindMarine     SourceKind = "marine"
	SourceKindAirQuality SourceKind = "air_quality"
)

func (k SourceKind) Valid() bool {
	switch k {
	case SourceKindWeather, SourceKindMarine, SourceKindAirQuality:
		return true
	default:
		return false
	}
}

// AllSourceKinds is the fixed reporting order.
func AllSourceKinds() []SourceKind {
	return []SourceKind{SourceKindWeather, SourceKindMarine, SourceKindAirQuality}
}

// SourceSlice is the normalized output of exactly one upstream source.
type SourceSlice struct {
	Kind      SourceKind
	Status    SourceStatus
	Hours     []WeatherObservation
	FetchedAt time.Time
	// ValidUntil is the last instant this source covers. It differs per source:
	// weather/marine reach the whole horizon, AQI is capped by upstream.
	ValidUntil time.Time
	// Geometry is filled by the weather source: it is the only source that
	// reports the model grid cell.
	Geometry ModelGeometry
}

// SourceCoverage describes how far one source reaches for one place. It lets a
// client see that, for example, AQI ends earlier than the weather forecast.
type SourceCoverage struct {
	Kind      SourceKind `json:"source"`
	Available bool       `json:"available"`
	Demo      bool       `json:"demo"`
	Reason    string     `json:"reason,omitempty"`
	From      string     `json:"from,omitempty"`
	To        string     `json:"to,omitempty"`
	Hours     int        `json:"hours"`
	Covered   int        `json:"covered_hours"`
	Fields    []Factor   `json:"fields"`
	Stale     bool       `json:"stale,omitempty"`
	FetchedAt string     `json:"fetched_at,omitempty"`
}

// WeatherSeries is the merged provider output: hourly observations plus
// provenance. It is assembled from independent source slices.
type WeatherSeries struct {
	PlaceID   string
	Hours     []WeatherObservation
	Sources   []SourceStatus
	Slices    []SourceSlice
	Geometry  ModelGeometry
	FetchedAt time.Time
}

// StoredSeries is what storage returns for one place over a time range: hours
// merged from every source, plus per-source coverage.
type StoredSeries struct {
	Observations []WeatherObservation
	Sources      []SourceStatus
	Geometry     ModelGeometry
	Coverage     []SourceCoverage
}

// SliceFor returns the normalized slice of one source, if present.
func (s WeatherSeries) SliceFor(kind SourceKind) (SourceSlice, bool) {
	for _, slice := range s.Slices {
		if slice.Kind == kind {
			return slice, true
		}
	}

	return SourceSlice{}, false
}

// MergeSlices combines independent source slices into one hourly series. Values
// are joined by UTC hour, never by array index.
func MergeSlices(placeID string, slices []SourceSlice) WeatherSeries {
	byHour := make(map[time.Time]modelsHour, 0)
	for _, slice := range slices {
		for _, obs := range slice.Hours {
			merged, ok := byHour[obs.Hour]
			if !ok {
				merged = modelsHour{hour: obs.Hour}
			}
			merged.obs = MergeObservations(merged.obs, obs, slice.Kind)
			merged.obs.Hour = obs.Hour
			byHour[obs.Hour] = merged
		}
	}

	hours := make([]time.Time, 0, len(byHour))
	for hour := range byHour {
		hours = append(hours, hour)
	}
	sortTimes(hours)

	result := make([]WeatherObservation, 0, len(hours))
	sources := make([]SourceStatus, 0, len(slices))
	for _, hour := range hours {
		result = append(result, byHour[hour].obs)
	}
	for _, slice := range slices {
		sources = append(sources, slice.Status)
	}

	return WeatherSeries{PlaceID: placeID, Hours: result, Sources: sources, Slices: slices}
}

type modelsHour struct {
	hour time.Time
	obs  WeatherObservation
}

// MergeObservations copies only the fields owned by the given source, so sources
// never overwrite each other's values.
func MergeObservations(dst, src WeatherObservation, kind SourceKind) WeatherObservation {
	switch kind {
	case SourceKindWeather:
		dst.AirTempC = src.AirTempC
		dst.ApparentTempC = src.ApparentTempC
		dst.PrecipMM = src.PrecipMM
		dst.WindSpeedMS = src.WindSpeedMS
		dst.WindGustMS = src.WindGustMS
		dst.WindDirection = src.WindDirection
		dst.HumidityPct = src.HumidityPct
		dst.CloudCoverPct = src.CloudCoverPct
		dst.VisibilityM = src.VisibilityM
		dst.WeatherCode = src.WeatherCode
	case SourceKindMarine:
		dst.WaveHeightM = src.WaveHeightM
		dst.WavePeriodS = src.WavePeriodS
		dst.WaveDirection = src.WaveDirection
		dst.WaterTempC = src.WaterTempC
		dst.SeaLevelHeightM = src.SeaLevelHeightM
	case SourceKindAirQuality:
		dst.AQIEuropean = src.AQIEuropean
		dst.PM25 = src.PM25
	}

	return dst
}

// FieldsOfKind lists the observation fields a source is responsible for.
func FieldsOfKind(kind SourceKind) []Factor {
	switch kind {
	case SourceKindWeather:
		return []Factor{
			FactorAirTemperature,
			FactorApparentTemperature,
			FactorWindSpeed,
			FactorWindGust,
			FactorPrecipitation,
			FactorVisibility,
			FactorWeatherCode,
		}
	case SourceKindMarine:
		return []Factor{FactorWaveHeight, FactorWaterTemperature}
	case SourceKindAirQuality:
		return []Factor{FactorAirQuality}
	default:
		return nil
	}
}

func sortTimes(items []time.Time) {
	sort.Slice(items, func(i, j int) bool { return items[i].Before(items[j]) })
}
