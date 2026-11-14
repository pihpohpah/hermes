// Package eventagent — клиент HTTP-сервиса скрапинга событий (webscraper):
// POST /runs запускает прогон, GET /runs/{id} отдаёт статус и результат.
package eventagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

const defaultTimeout = 30 * time.Second

// Config configures the event-agent client. Zero BaseURL means the agent is
// expected on the loopback default; zero Timeout falls back to defaultTimeout.
type Config struct {
	BaseURL string
	Timeout time.Duration
	Logger  *slog.Logger
	Client  *http.Client
}

type Client struct {
	cfg    Config
	logger *slog.Logger
}

func NewClient(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: cfg.Timeout}
	}

	return &Client{cfg: cfg, logger: cfg.Logger}
}

func (c *Client) base() string {
	if c.cfg.BaseURL != "" {
		return strings.TrimRight(c.cfg.BaseURL, "/")
	}

	return "http://127.0.0.1:8000"
}

// StartRun запускает прогон агента. 409 (прогон занят другим процессом) —
// это models.ErrConflict, а не сбой транспорта.
func (c *Client) StartRun(ctx context.Context) (models.ImportedRun, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base()+"/runs", nil)
	if err != nil {
		return models.ImportedRun{}, fmt.Errorf("build start run request: %w", err)
	}
	req.Header.Set("User-Agent", "hermes-events-import/1.0")

	resp, err := c.cfg.Client.Do(req)
	if err != nil {
		return models.ImportedRun{}, fmt.Errorf("start run request: %w", err)
	}
	defer c.closeBody(resp)

	if resp.StatusCode == http.StatusConflict {
		return models.ImportedRun{}, fmt.Errorf("start run: %w", models.ErrConflict)
	}
	if resp.StatusCode != http.StatusAccepted {
		return models.ImportedRun{}, fmt.Errorf("start run: unexpected status %d", resp.StatusCode)
	}

	run, err := decodeRun(resp)
	if err != nil {
		return models.ImportedRun{}, fmt.Errorf("decode start run response: %w", err)
	}

	return run, nil
}

// GetRun возвращает состояние прогона; пока он running, Events ещё пуст.
func (c *Client) GetRun(ctx context.Context, runID string) (models.ImportedRun, error) {
	if strings.TrimSpace(runID) == "" {
		return models.ImportedRun{}, errors.New("get run: empty run id")
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, c.base()+"/runs/"+url.PathEscape(runID), nil,
	)
	if err != nil {
		return models.ImportedRun{}, fmt.Errorf("build get run request: %w", err)
	}
	req.Header.Set("User-Agent", "hermes-events-import/1.0")

	resp, err := c.cfg.Client.Do(req)
	if err != nil {
		return models.ImportedRun{}, fmt.Errorf("get run request: %w", err)
	}
	defer c.closeBody(resp)

	if resp.StatusCode == http.StatusNotFound {
		return models.ImportedRun{}, fmt.Errorf("get run `%s`: %w", runID, models.ErrNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return models.ImportedRun{}, fmt.Errorf("get run `%s`: unexpected status %d", runID, resp.StatusCode)
	}

	run, err := decodeRun(resp)
	if err != nil {
		return models.ImportedRun{}, fmt.Errorf("decode get run response: %w", err)
	}

	return run, nil
}

func (c *Client) closeBody(resp *http.Response) {
	if err := resp.Body.Close(); err != nil {
		c.logger.Warn("close event-agent response body", slog.Any("error", err))
	}
}

const maxRunBodyBytes = 8 << 20

func decodeRun(resp *http.Response) (models.ImportedRun, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRunBodyBytes))
	if err != nil {
		return models.ImportedRun{}, fmt.Errorf("read run response: %w", err)
	}

	var dto runResponse
	if err := json.Unmarshal(body, &dto); err != nil {
		return models.ImportedRun{}, fmt.Errorf("unmarshal run response: %w", err)
	}

	run, err := dto.toDomain()
	if err != nil {
		return models.ImportedRun{}, fmt.Errorf("map run response: %w", err)
	}

	return run, nil
}
