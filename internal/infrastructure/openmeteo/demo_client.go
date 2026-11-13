package openmeteo

import (
	"context"
	"hash/fnv"
	"math"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// HourlyProvider is the provider contract consumed by the application layer.
// Sources are fetched independently so one failure or a shorter horizon cannot
// degrade the others.
type HourlyProvider interface {
	FetchHourly(ctx context.Context, place models.Place, rng models.ForecastRange) (models.WeatherSeries, error)
	FetchSource(
		ctx context.Context,
		place models.Place,
		kind models.SourceKind,
		rng models.ForecastRange,
	) (models.SourceSlice, error)
}

// DemoProvider generates deterministic fake weather. It is only used when the
// operator explicitly selects the demo mode; a failing real API never falls
// back to it.
type DemoProvider struct{}

func NewDemoProvider() *DemoProvider {
	return &DemoProvider{}
}

const (
	defaultElevationM  = 15
	dayPhaseOffsetHour = 6
	baseAirTempC       = 12.5
	airTempSwingC      = 4.5
	lapseRatePerMeter  = 0.006
	baseWaterTempC     = 14.0
	waterTempSwingC    = 0.8
	baseWindSpeedMS    = 3.2
	windSwingMS        = 2.1
	gustMultiplier     = 1.35
	baseWaveHeightM    = 0.35
	waveSwingM         = 0.25
	baseSeaLevelM      = 0.6
	seaLevelSwingM     = 0.45
	baseAQI            = 22
	aqiModulo          = 9
	pm25PerAQIRatio    = 0.35
	radiansPerDayCycle = 2 * math.Pi / 24.0
	radiansPerTide     = 2 * math.Pi / 12.4
	hashBuckets        = 100
	windPlaceSpreadMS  = 1.4
	wavePlaceSpreadM   = 0.22
	tempPlaceSpreadC   = 1.6
	phaseSpreadHours   = 3.0
	minWindMS          = 0.5
	minWaveM           = 0.05
	jitterSpan         = 2.0
	round1Factor       = 10.0
	round2Factor       = 100.0

	apparentTempOffsetC = 0.6
	degreesPerHour      = 15
	fullCircleDegrees   = 360.0
	baseHumidityPct     = 60
	humiditySpanPct     = 20
	baseCloudCoverPct   = 20
	cloudSpanPct        = 10
	cloudModulo         = 5
	demoVisibilityM     = 20000.0
	demoWavePeriodS     = 4.2
	demoWaveDirection   = 180.0
)

// FetchSource returns one demo source slice covering the whole rolling range.
func (d *DemoProvider) FetchSource(
	ctx context.Context,
	place models.Place,
	kind models.SourceKind,
	rng models.ForecastRange,
) (models.SourceSlice, error) {
	series, err := d.FetchHourly(ctx, place, rng)
	if err != nil {
		return models.SourceSlice{}, err
	}
	for _, slice := range series.Slices {
		if slice.Kind == kind {
			return slice, nil
		}
	}

	return models.SourceSlice{}, nil
}

// FetchHourly builds the full demo series for the rolling range.
func (d *DemoProvider) FetchHourly(
	_ context.Context,
	place models.Place,
	rng models.ForecastRange,
) (models.WeatherSeries, error) {
	elev := defaultElevationM
	if place.Elevation != nil {
		elev = *place.Elevation
	}

	placeOffset := placeJitter(place.ID)
	phaseShift := placeOffset * phaseSpreadHours

	hours := make([]models.WeatherObservation, 0, rng.Hours())
	for instant := rng.From.UTC().Truncate(time.Hour); instant.Before(rng.To); instant = instant.Add(time.Hour) {
		local := instant.In(rng.Location)
		hour := local.Hour()
		dayPhase := math.Sin((float64(hour-dayPhaseOffsetHour) + phaseShift) * radiansPerDayCycle)
		tidePhase := math.Sin((float64(hour) + phaseShift) * radiansPerTide)

		wind := round1(max(minWindMS, baseWindSpeedMS+placeOffset*windPlaceSpreadMS+windSwingMS*math.Max(0, dayPhase)))
		gust := round1(wind * gustMultiplier)
		wave := round2(max(minWaveM, baseWaveHeightM+placeOffset*wavePlaceSpreadM+waveSwingM*math.Max(0, dayPhase)))
		seaLevel := round2(baseSeaLevelM + seaLevelSwingM*tidePhase)
		airTemp := round1(
			baseAirTempC + placeOffset*tempPlaceSpreadC + airTempSwingC*dayPhase - float64(elev)*lapseRatePerMeter,
		)
		waterTemp := round1(baseWaterTempC + placeOffset*waterTempSwingC + waterTempSwingC*dayPhase)
		aqi := baseAQI + ((hour + int(math.Abs(placeOffset)*aqiModulo)) % aqiModulo)

		hours = append(hours, models.WeatherObservation{
			Hour:            instant,
			AirTempC:        models.NewMeasure(airTemp),
			ApparentTempC:   models.NewMeasure(round1(airTemp - apparentTempOffsetC)),
			PrecipMM:        models.NewMeasure(0),
			WindSpeedMS:     models.NewMeasure(wind),
			WindGustMS:      models.NewMeasure(gust),
			WindDirection:   models.NewMeasure(math.Mod(float64(hour*degreesPerHour), fullCircleDegrees)),
			HumidityPct:     models.NewMeasure(float64(baseHumidityPct + hour%humiditySpanPct)),
			CloudCoverPct:   models.NewMeasure(float64(baseCloudCoverPct + (hour%cloudModulo)*cloudSpanPct)),
			VisibilityM:     models.NewMeasure(demoVisibilityM),
			WeatherCode:     models.NewMeasure(1),
			WaveHeightM:     models.NewMeasure(wave),
			WavePeriodS:     models.NewMeasure(demoWavePeriodS),
			WaveDirection:   models.NewMeasure(demoWaveDirection),
			WaterTempC:      models.NewMeasure(waterTemp),
			SeaLevelHeightM: models.NewMeasure(seaLevel),
			AQIEuropean:     models.NewMeasure(float64(aqi)),
			PM25:            models.NewMeasure(round1(float64(aqi) * pm25PerAQIRatio)),
		})
	}

	sources := []models.SourceStatus{
		{Source: models.SourceOpenMeteoWeather, Available: true, Demo: true},
		{Source: models.SourceOpenMeteoMarine, Available: true, Demo: true},
		{Source: models.SourceOpenMeteoAirQuality, Available: true, Demo: true},
	}
	slices := make([]models.SourceSlice, 0, len(sources))
	for _, kind := range models.AllSourceKinds() {
		for _, source := range sources {
			if models.SourceIDForKind(kind) != source.Source {
				continue
			}
			slices = append(slices, models.SourceSlice{
				Kind:       kind,
				Status:     source,
				Hours:      hours,
				ValidUntil: rng.To,
				FetchedAt:  time.Now().UTC(),
				Geometry:   models.ModelGeometry{},
			})
		}
	}

	series := models.MergeSlices(place.ID, slices)
	series.FetchedAt = time.Now().UTC()

	return series, nil
}

func placeJitter(placeID string) float64 {
	h := fnv.New32a()
	if _, err := h.Write([]byte(placeID)); err != nil {
		return 0
	}
	bucket := float64(h.Sum32() % hashBuckets)

	return (bucket/hashBuckets)*jitterSpan - 1.0
}

func round1(v float64) float64 {
	return math.Round(v*round1Factor) / round1Factor
}

func round2(v float64) float64 {
	return math.Round(v*round2Factor) / round2Factor
}

// compile-time check that the demo provider satisfies the provider contract.
var _ HourlyProvider = (*DemoProvider)(nil)
