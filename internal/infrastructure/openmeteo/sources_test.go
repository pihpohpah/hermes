package openmeteo_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
	"github.com/pihpohpah/hermes/internal/infrastructure/openmeteo"
)

func testPlace() models.Place {
	return models.Place{
		ID:       "11111111-1111-1111-1111-111111111111",
		Type:     models.PlaceTypeSupSpot,
		Layer:    models.LayerSea,
		Location: models.Point{Lat: 43.0732, Lon: 131.8431},
	}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}

	return raw
}

func rangeFor(t *testing.T) models.ForecastRange {
	t.Helper()

	rng, err := models.NewForecastRange(time.Date(2026, time.November, 12, 14, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("build forecast range: %v", err)
	}

	return rng
}

func fixtureServer(t *testing.T, paths *[]string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*paths = append(*paths, r.URL.Path+"?"+r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/v1/forecast":
			_, _ = w.Write(fixture(t, "weather.json"))
		case "/v1/marine":
			_, _ = w.Write(fixture(t, "marine.json"))
		case "/v1/air-quality":
			_, _ = w.Write(fixture(t, "air_quality.json"))
		default:
			http.NotFound(w, r)
		}
	}))
}

func newClient(server *httptest.Server, mutate func(*openmeteo.Config)) *openmeteo.Client {
	cfg := openmeteo.Config{
		WeatherBaseURL:    server.URL + "/v1",
		MarineBaseURL:     server.URL + "/v1",
		AirQualityBaseURL: server.URL + "/v1",
		Timeout:           2 * time.Second,
		Attempts:          1,
	}
	if mutate != nil {
		mutate(&cfg)
	}

	return openmeteo.NewClient(cfg)
}

func TestFetchHourlyJoinsSourcesByTimestamp(t *testing.T) {
	t.Parallel()

	var paths []string
	server := fixtureServer(t, &paths)
	defer server.Close()

	series, err := newClient(server, nil).FetchHourly(context.Background(), testPlace(), rangeFor(t))
	if err != nil {
		t.Fatalf("fetch hourly: %v", err)
	}

	first, ok := series.At(time.Date(2026, 11, 12, 14, 0, 0, 0, time.UTC))
	if !ok {
		t.Fatal("hour 14:00 missing from the merged series")
	}

	assertMeasure(t, "air temp", first.AirTempC, 6.4)
	assertMeasure(t, "wind speed m/s", first.WindSpeedMS, 2.0)
	assertMeasure(t, "wave height", first.WaveHeightM, 0.42)
	assertMeasure(t, "water temp", first.WaterTempC, 12.4)
	assertMeasure(t, "aqi", first.AQIEuropean, 11)

	third, ok := series.At(time.Date(2026, 11, 12, 16, 0, 0, 0, time.UTC))
	if !ok {
		t.Fatal("hour 16:00 missing")
	}
	if third.WaveHeightM.Known {
		t.Fatalf("null wave height must stay unknown, got %v", third.WaveHeightM.Value)
	}
	if !third.AirTempC.Known {
		t.Fatal("air temp at 16:00 is present in the fixture")
	}

	fourth, ok := series.At(time.Date(2026, 11, 12, 17, 0, 0, 0, time.UTC))
	if !ok {
		t.Fatal("hour 17:00 missing")
	}
	if fourth.AQIEuropean.Known {
		t.Fatal("null AQI must stay unknown")
	}
	if !fourth.WaveHeightM.Known {
		t.Fatal("wave height at 17:00 is present in the fixture")
	}

	if len(series.Sources) != 3 {
		t.Fatalf("expected 3 source statuses, got %d", len(series.Sources))
	}
	for _, source := range series.Sources {
		if !source.Available {
			t.Fatalf("source %s should be available: %s", source.Source, source.Reason)
		}
		if source.Demo {
			t.Fatalf("real provider must not be marked demo: %s", source.Source)
		}
	}

	if len(paths) == 0 {
		t.Fatal("no requests recorded")
	}
	wantParam(t, paths[0], "timezone", "UTC")
}

func TestMarineFailureDoesNotBreakForecast(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/marine" {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":true,"reason":"service unavailable"}`))

			return
		}
		switch r.URL.Path {
		case "/v1/forecast":
			_, _ = w.Write(fixture(t, "weather.json"))
		default:
			_, _ = w.Write(fixture(t, "air_quality.json"))
		}
	}))
	defer server.Close()

	series, err := newClient(server, nil).FetchHourly(context.Background(), testPlace(), rangeFor(t))
	if err != nil {
		t.Fatalf("marine outage must not fail the whole forecast: %v", err)
	}

	var marine models.SourceStatus
	for _, source := range series.Sources {
		if source.Source == models.SourceOpenMeteoMarine {
			marine = source
		}
	}
	if marine.Available {
		t.Fatal("marine source must be reported unavailable")
	}
	if marine.Reason == "" {
		t.Fatal("unavailable source must explain itself to the client")
	}

	at, ok := series.At(time.Date(2026, 11, 12, 14, 0, 0, 0, time.UTC))
	if !ok {
		t.Fatal("hour 14:00 missing")
	}
	if at.WaveHeightM.Known {
		t.Fatal("wave height must be unknown when marine source failed")
	}
	if !at.WindSpeedMS.Known {
		t.Fatal("weather values must survive a marine outage")
	}
}

func TestAirQualityFailureIsIsolated(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/air-quality" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":true,"reason":"bad request"}`))

			return
		}
		if r.URL.Path == "/v1/marine" {
			_, _ = w.Write(fixture(t, "marine.json"))

			return
		}
		_, _ = w.Write(fixture(t, "weather.json"))
	}))
	defer server.Close()

	series, err := newClient(server, nil).FetchHourly(context.Background(), testPlace(), rangeFor(t))
	if err != nil {
		t.Fatalf("AQI failure must not break the forecast: %v", err)
	}

	at, ok := series.At(time.Date(2026, 11, 12, 14, 0, 0, 0, time.UTC))
	if !ok {
		t.Fatal("hour 14:00 missing")
	}
	if at.AQIEuropean.Known {
		t.Fatal("AQI must stay unknown when the source failed")
	}
	if !at.WaveHeightM.Known {
		t.Fatal("marine data must still be joined")
	}
}

func TestWeatherFailureFailsFast(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":true,"reason":"boom"}`))
	}))
	defer server.Close()

	_, err := newClient(server, nil).FetchHourly(context.Background(), testPlace(), rangeFor(t))
	if err == nil {
		t.Fatal("weather source failure must fail the request")
	}
	if !errors.Is(err, openmeteo.ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}

func TestRetrySucceedsAfterTemporaryError(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if attempts == 0 {
			attempts++
			w.WriteHeader(http.StatusBadGateway)

			return
		}
		attempts++
		switch r.URL.Path {
		case "/v1/forecast":
			_, _ = w.Write(fixture(t, "weather.json"))
		case "/v1/marine":
			_, _ = w.Write(fixture(t, "marine.json"))
		default:
			_, _ = w.Write(fixture(t, "air_quality.json"))
		}
	}))
	defer server.Close()

	client := newClient(server, func(c *openmeteo.Config) {
		c.Attempts = 2
		c.BaseBackoff = time.Millisecond
	})
	slept := 0
	client.SetSleepForTest(func(context.Context, time.Duration) error {
		slept++

		return nil
	})

	series, err := client.FetchHourly(context.Background(), testPlace(), rangeFor(t))
	if err != nil {
		t.Fatalf("retry must recover from a temporary error: %v", err)
	}
	if len(series.Hours) == 0 {
		t.Fatal("expected data after retry")
	}
	if slept == 0 {
		t.Fatal("expected a backoff before the retry")
	}
}

func TestRateLimitIsReported(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":true,"reason":"rate limited"}`))
	}))
	defer server.Close()

	client := newClient(server, func(c *openmeteo.Config) {
		c.Attempts = 2
		c.Timeout = time.Second
	})
	client.SetSleepForTest(func(context.Context, time.Duration) error { return nil })

	_, err := client.FetchHourly(context.Background(), testPlace(), rangeFor(t))
	if !errors.Is(err, openmeteo.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
	if requests > 2 {
		t.Fatalf("retry budget exceeded: %d requests", requests)
	}
}

func TestTimeoutIsBounded(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)

	client := newClient(server, func(c *openmeteo.Config) {
		c.Timeout = 30 * time.Millisecond
		c.Attempts = 1
	})

	start := time.Now()
	_, err := client.FetchHourly(context.Background(), testPlace(), rangeFor(t))
	elapsed := time.Since(start)

	if !errors.Is(err, openmeteo.ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable on timeout, got %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("timeout was not enforced, took %s", elapsed)
	}
}

func TestInlandPlaceSkipsMarine(t *testing.T) {
	t.Parallel()

	var paths []string
	server := fixtureServer(t, &paths)
	defer server.Close()

	inland := models.Place{
		ID:       "22222222-2222-2222-2222-222222222222",
		Type:     models.PlaceTypePark,
		Layer:    models.LayerCity,
		Location: models.Point{Lat: 43.1186, Lon: 131.9082},
	}

	series, err := newClient(server, nil).FetchHourly(context.Background(), inland, rangeFor(t))
	if err != nil {
		t.Fatalf("fetch inland: %v", err)
	}

	for _, path := range paths {
		if strings.Contains(path, "/v1/marine") {
			t.Fatalf("marine API must not be queried for inland places: %s", path)
		}
	}

	at, ok := series.At(time.Date(2026, 11, 12, 14, 0, 0, 0, time.UTC))
	if !ok {
		t.Fatal("hour 14:00 missing")
	}
	if at.WaveHeightM.Known {
		t.Fatal("inland places must have no wave data")
	}
}

func TestDemoProviderIsMarkedAsDemo(t *testing.T) {
	t.Parallel()

	series, err := openmeteo.NewDemoProvider().FetchHourly(context.Background(), testPlace(), rangeFor(t))
	if err != nil {
		t.Fatalf("demo fetch: %v", err)
	}

	expected := rangeFor(t).Hours()
	if len(series.Hours) != expected {
		t.Fatalf("expected %d demo hours, got %d", expected, len(series.Hours))
	}
	if !series.IsDemo() {
		t.Fatal("demo provider output must be marked as demo")
	}
}

func TestSourcesAreFetchedIndependently(t *testing.T) {
	t.Parallel()

	seen := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path]++
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/v1/forecast":
			_, _ = w.Write(fixture(t, "weather.json"))
		case "/v1/marine":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":true,"reason":"down"}`))
		default:
			_, _ = w.Write(fixture(t, "air_quality.json"))
		}
	}))
	defer server.Close()

	client := newClient(server, nil)
	at := rangeFor(t)

	for _, kind := range models.AllSourceKinds() {
		if _, err := client.FetchSource(context.Background(), testPlace(), kind, at); err != nil {
			t.Fatalf("source %s must not fail the request: %v", kind, err)
		}
	}

	for _, path := range []string{"/v1/forecast", "/v1/marine", "/v1/air-quality"} {
		if seen[path] == 0 {
			t.Fatalf("source endpoint %s was never requested", path)
		}
	}
}

func TestMarineFailureDoesNotMarkWeatherUnavailable(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/marine" {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":true}`))

			return
		}
		if r.URL.Path == "/v1/forecast" {
			_, _ = w.Write(fixture(t, "weather.json"))

			return
		}
		_, _ = w.Write(fixture(t, "air_quality.json"))
	}))
	defer server.Close()

	series, err := newClient(server, nil).FetchHourly(context.Background(), testPlace(), rangeFor(t))
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	states := make(map[models.SourceKind]bool)
	for _, source := range series.Sources {
		for _, kind := range models.AllSourceKinds() {
			if models.SourceIDForKind(kind) == source.Source {
				states[kind] = source.Available
			}
		}
	}
	if !states[models.SourceKindWeather] {
		t.Fatal("weather source must stay available when marine fails")
	}
	if !states[models.SourceKindAirQuality] {
		t.Fatal("air quality must stay available when marine fails")
	}
	if states[models.SourceKindMarine] {
		t.Fatal("marine source must be reported unavailable")
	}
}

func TestAQIShortHorizonDoesNotTruncateWeather(t *testing.T) {
	t.Parallel()

	var aqiParams string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/air-quality":
			aqiParams = r.URL.RawQuery
			_, _ = w.Write(fixture(t, "air_quality.json"))
		case "/v1/marine":
			_, _ = w.Write(fixture(t, "marine.json"))
		default:
			_, _ = w.Write(fixture(t, "weather.json"))
		}
	}))
	defer server.Close()

	series, err := newClient(server, nil).FetchHourly(context.Background(), testPlace(), rangeFor(t))
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	weather, ok := series.SliceFor(models.SourceKindWeather)
	if !ok {
		t.Fatal("weather slice missing")
	}
	aqi, ok := series.SliceFor(models.SourceKindAirQuality)
	if !ok {
		t.Fatal("air quality slice missing")
	}

	if !aqi.ValidUntil.Before(weather.ValidUntil) {
		t.Fatalf("AQI must cover less than weather: aqi until %s, weather until %s",
			aqi.ValidUntil, weather.ValidUntil)
	}
	if aqiParams == "" {
		t.Fatal("AQI request was not made")
	}
}

func TestFetchElevationsPreservesOrderAndUnknowns(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("latitude") == "" {
			t.Error("latitude must be sent")
		}
		_, _ = w.Write([]byte(`{"elevation":[145,null,3]}`))
	}))
	defer server.Close()

	client := openmeteo.NewElevationClient(openmeteo.Config{
		WeatherBaseURL: server.URL,
		Timeout:        time.Second,
		Attempts:       1,
	})

	points := []models.Point{
		{Lat: 43.1186, Lon: 131.9082},
		{Lat: 43.1003, Lon: 131.9061},
		{Lat: 43.1636, Lon: 132.0497},
	}
	values, err := client.FetchElevations(context.Background(), points)
	if err != nil {
		t.Fatalf("fetch elevations: %v", err)
	}
	if len(values) != 3 {
		t.Fatalf("got %d values, want 3", len(values))
	}
	if !values[0].Known || values[0].Value != 145 {
		t.Fatalf("first value = %+v, want 145", values[0])
	}
	if values[1].Known {
		t.Fatalf("null elevation must stay unknown, got %v", values[1].Value)
	}
	if !values[2].Known || values[2].Value != 3 {
		t.Fatalf("third value = %+v, want 3", values[2])
	}
}

func TestFetchElevationsBatchesLargeSets(t *testing.T) {
	t.Parallel()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		parts := splitNonEmpty(r.URL.Query().Get("latitude"))
		payload := make([]float64, 0, len(parts))
		for range parts {
			payload = append(payload, 100)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mustJSON(map[string]any{"elevation": payload}))
	}))
	defer server.Close()

	client := openmeteo.NewElevationClient(openmeteo.Config{
		WeatherBaseURL: server.URL,
		Timeout:        time.Second,
		Attempts:       1,
	})

	points := make([]models.Point, 250)
	for i := range points {
		points[i] = models.Point{Lat: 43.0 + float64(i)*0.001, Lon: 131.8}
	}
	values, err := client.FetchElevations(context.Background(), points)
	if err != nil {
		t.Fatalf("fetch elevations: %v", err)
	}
	if len(values) != 250 {
		t.Fatalf("got %d values, want 250", len(values))
	}
	if requests < 3 {
		t.Fatalf("expected the set to be split into batches, got %d request(s)", requests)
	}
}

func TestFetchElevationsReportsCountMismatch(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"elevation":[10]}`))
	}))
	defer server.Close()

	client := openmeteo.NewElevationClient(openmeteo.Config{
		WeatherBaseURL: server.URL,
		Timeout:        time.Second,
		Attempts:       1,
	})

	_, err := client.FetchElevations(context.Background(), []models.Point{
		{Lat: 43.0, Lon: 131.8},
		{Lat: 43.1, Lon: 131.9},
	})
	if !errors.Is(err, openmeteo.ErrInvalidResponse) {
		t.Fatalf("expected ErrInvalidResponse, got %v", err)
	}
}

func TestGeometryRecordsCoarseModelCaveat(t *testing.T) {
	t.Parallel()

	var paths []string
	server := fixtureServer(t, &paths)
	defer server.Close()

	elevation := 145
	place := models.Place{
		ID:        "33333333-3333-3333-3333-333333333333",
		Type:      models.PlaceTypePark,
		Layer:     models.LayerCity,
		Location:  models.Point{Lat: 43.1186, Lon: 131.9082},
		Elevation: &elevation,
	}

	series, err := newClient(server, nil).FetchHourly(context.Background(), place, rangeFor(t))
	if err != nil {
		t.Fatalf("fetch hourly: %v", err)
	}

	if !series.Geometry.PlaceElevationM.Known || series.Geometry.PlaceElevationM.Value != 145 {
		t.Fatalf("place elevation not recorded: %+v", series.Geometry.PlaceElevationM)
	}
	if series.Geometry.Terrain != string(models.TerrainHilly) {
		t.Fatalf("terrain = %q, want hilly", series.Geometry.Terrain)
	}
}

func splitNonEmpty(raw string) []string {
	if raw == "" {
		return nil
	}

	return strings.Split(raw, ",")
}

func mustJSON(v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}

	return raw
}

func assertMeasure(t *testing.T, name string, m models.Measure, want float64) {
	t.Helper()

	if !m.Known {
		t.Fatalf("%s: value is unknown, want %v", name, want)
	}
	if diff := m.Value - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("%s = %v, want %v", name, m.Value, want)
	}
}

func wantParam(t *testing.T, rawQuery, key, want string) {
	t.Helper()

	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		t.Fatalf("parse query %q: %v", rawQuery, err)
	}
	if got := values.Get(key); got != want {
		t.Fatalf("query param %s = %q, want %q", key, got, want)
	}
}
