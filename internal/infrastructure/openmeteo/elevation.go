package openmeteo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// maxPointsPerElevationRequest is the largest batch accepted by the elevation
// endpoint in practice; larger sets are split.
const maxPointsPerElevationRequest = 100

// ElevationClient resolves ground elevation for many coordinates at once.
type ElevationClient struct {
	cfg Config
}

func NewElevationClient(cfg Config) *ElevationClient {
	return &ElevationClient{cfg: cfg.withDefaults()}
}

type elevationResponse struct {
	Elevation []*float64 `json:"elevation"`
	Error     bool       `json:"error"`
	Reason    string     `json:"reason"`
}

// FetchElevations returns one elevation per point, in the same order. Points
// the API reports as null stay unknown; nothing is replaced with zero.
func (c *ElevationClient) FetchElevations(
	ctx context.Context,
	points []models.Point,
) ([]models.Measure, error) {
	result := make([]models.Measure, len(points))
	for start := 0; start < len(points); start += maxPointsPerElevationRequest {
		end := min(start+maxPointsPerElevationRequest, len(points))
		batch, err := c.fetchBatch(ctx, points[start:end])
		if err != nil {
			return nil, err
		}
		copy(result[start:end], batch)
	}

	return result, nil
}

func (c *ElevationClient) fetchBatch(ctx context.Context, points []models.Point) ([]models.Measure, error) {
	latitudes := make([]string, 0, len(points))
	longitudes := make([]string, 0, len(points))
	for _, point := range points {
		latitudes = append(latitudes, strconv.FormatFloat(point.Lat, 'f', 5, 64))
		longitudes = append(longitudes, strconv.FormatFloat(point.Lon, 'f', 5, 64))
	}

	query := url.Values{}
	query.Set("latitude", strings.Join(latitudes, ","))
	query.Set("longitude", strings.Join(longitudes, ","))
	if c.cfg.APIKey != "" {
		query.Set("apikey", c.cfg.APIKey)
	}

	var (
		response elevationResponse
		err      error
	)
	target := c.cfg.WeatherBaseURL + "/elevation?" + query.Encode()
	for attempt := 1; attempt <= c.cfg.Attempts; attempt++ {
		response, err = c.doElevation(ctx, target)
		if err == nil {
			break
		}
		if !isRetryable(err) || attempt == c.cfg.Attempts {
			break
		}
		if waitErr := sleep(ctx, c.cfg.BaseBackoff<<(attempt-1)); waitErr != nil {
			return nil, fmt.Errorf("retry elevation request: %w", waitErr)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("fetch elevations: %w", err)
	}
	if response.Error {
		return nil, fmt.Errorf("%w: %s", ErrInvalidResponse, response.Reason)
	}
	if len(response.Elevation) != len(points) {
		return nil, fmt.Errorf(
			"%w: asked for %d elevations, got %d",
			ErrInvalidResponse,
			len(points),
			len(response.Elevation),
		)
	}

	result := make([]models.Measure, len(points))
	for i, value := range response.Elevation {
		if value == nil {
			result[i] = models.MissingMeasure()

			continue
		}
		result[i] = models.NewMeasure(*value)
	}

	return result, nil
}

func (c *ElevationClient) doElevation(ctx context.Context, target string) (elevationResponse, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, target, nil)
	if err != nil {
		return elevationResponse{}, fmt.Errorf("%w: build request: %w", ErrInvalidResponse, err)
	}
	req.Header.Set("User-Agent", "hermes-weather/"+providerName)
	req.Header.Set("Accept", "application/json")

	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return elevationResponse{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if readErr != nil {
		return elevationResponse{}, fmt.Errorf("%w: read body: %w", ErrUnavailable, readErr)
	}

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return elevationResponse{}, &statusError{
			kind:       ErrRateLimited,
			statusCode: resp.StatusCode,
			reason:     "too many requests",
			retryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()),
		}
	case resp.StatusCode >= http.StatusInternalServerError:
		return elevationResponse{}, &statusError{
			kind:       ErrUnavailable,
			statusCode: resp.StatusCode,
			reason:     truncate(string(body), errorBodyLimit),
		}
	case resp.StatusCode != http.StatusOK:
		return elevationResponse{}, &statusError{
			kind:       ErrInvalidResponse,
			statusCode: resp.StatusCode,
			reason:     truncate(string(body), errorBodyLimit),
		}
	}

	var decoded elevationResponse
	if decodeErr := json.Unmarshal(body, &decoded); decodeErr != nil {
		return elevationResponse{}, fmt.Errorf("%w: decode body: %w", ErrInvalidResponse, decodeErr)
	}

	return decoded, nil
}
