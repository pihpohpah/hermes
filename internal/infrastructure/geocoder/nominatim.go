// Package geocoder превращает свободный текст адреса площадки в координаты.
// Реализация по умолчанию — публичный Nominatim (OSM): без ключа, но с
// обязательным User-Agent и ограничением не быстрее одного запроса в
// секунду; запросы идут строго последовательно.
package geocoder

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

const (
	defaultBaseURL     = "https://nominatim.openstreetmap.org"
	defaultTimeout     = 10 * time.Second
	defaultMinInterval = 1100 * time.Millisecond
	defaultUserAgent   = "hermes-events-import/1.0"

	maxResponseBytes = 1 << 20
)

// Config configures the Nominatim geocoder. BaseURL allows swapping the
// public instance for a self-hosted or commercial Nominatim-compatible one.
type Config struct {
	BaseURL     string
	Timeout     time.Duration
	UserAgent   string
	MinInterval time.Duration
	Client      *http.Client
}

type Nominatim struct {
	cfg Config

	mu       sync.Mutex
	lastCall time.Time
}

func NewNominatim(cfg Config) *Nominatim {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = defaultUserAgent
	}
	if cfg.MinInterval <= 0 {
		cfg.MinInterval = defaultMinInterval
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: cfg.Timeout}
	}

	return &Nominatim{cfg: cfg}
}

type searchResult struct {
	Lat string `json:"lat"`
	Lon string `json:"lon"`
}

// Geocode возвращает координаты первого результата поиска. Пустая выдача —
// models.ErrNotFound: у события просто не будет координат, импорт продолжается.
func (n *Nominatim) Geocode(ctx context.Context, address string) (models.Point, error) {
	if err := n.throttle(ctx); err != nil {
		return models.Point{}, err
	}

	query := url.Values{}
	query.Set("q", address)
	query.Set("format", "jsonv2")
	query.Set("limit", "1")
	query.Set("countrycodes", "ru")
	// Мягкий сдвиг ранжирования на Владивосток и окраины (bounded=0).
	query.Set("viewbox", "131.0,43.6,132.6,42.7")
	query.Set("bounded", "0")

	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, n.cfg.BaseURL+"/search?"+query.Encode(), nil,
	)
	if err != nil {
		return models.Point{}, fmt.Errorf("build geocode request: %w", err)
	}
	req.Header.Set("User-Agent", n.cfg.UserAgent)

	resp, err := n.cfg.Client.Do(req)
	if err != nil {
		return models.Point{}, fmt.Errorf("geocode request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return models.Point{}, fmt.Errorf("geocode request: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return models.Point{}, fmt.Errorf("read geocode response: %w", err)
	}

	var results []searchResult
	if err := json.Unmarshal(body, &results); err != nil {
		return models.Point{}, fmt.Errorf("unmarshal geocode response: %w", err)
	}
	if len(results) == 0 {
		return models.Point{}, fmt.Errorf("geocode `%s`: %w", address, models.ErrNotFound)
	}

	lat, err := strconv.ParseFloat(results[0].Lat, 64)
	if err != nil {
		return models.Point{}, fmt.Errorf("parse geocode lat `%s`: %w", results[0].Lat, err)
	}
	lon, err := strconv.ParseFloat(results[0].Lon, 64)
	if err != nil {
		return models.Point{}, fmt.Errorf("parse geocode lon `%s`: %w", results[0].Lon, err)
	}

	point, err := models.NewPoint(lat, lon)
	if err != nil {
		return models.Point{}, fmt.Errorf("geocode result for `%s`: %w", address, err)
	}

	return point, nil
}

// throttle держит паузу MinInterval между запросами — публичный Nominatim
// разрешает максимум один запрос в секунду.
func (n *Nominatim) throttle(ctx context.Context) error {
	n.mu.Lock()
	elapsed := time.Since(n.lastCall)
	if wait := n.cfg.MinInterval - elapsed; wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			n.mu.Unlock()

			return fmt.Errorf("geocoder throttle: %w", ctx.Err())
		case <-timer.C:
		}
	}
	n.lastCall = time.Now()
	n.mu.Unlock()

	return nil
}
