package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultBaseURL     = "https://api.telegram.org"
	defaultMaxAttempts = 6
	defaultTimeout     = 15 * time.Second
	retryBackoff       = 300 * time.Millisecond
	errBodyLimit       = 200
)

var ErrNoToken = errors.New("telegram bot token is not configured")

// Config собирает клиента из конфигурации приложения.
type Config struct {
	// BaseURL по умолчанию https://api.telegram.org; переопределяется в тестах.
	BaseURL string
	Token   string
	// Proxies — список "http://user:pass@host:port". Пусто = прямые соединения (каждая попытка заново).
	Proxies     []*url.URL
	MaxAttempts int
	Timeout     time.Duration
}

func (c Config) attempts() int {
	if c.MaxAttempts <= 0 {
		return defaultMaxAttempts
	}

	return c.MaxAttempts
}

func (c Config) timeout() time.Duration {
	if c.Timeout <= 0 {
		return defaultTimeout
	}

	return c.Timeout
}

func (c Config) baseURL() string {
	if strings.TrimRight(c.BaseURL, "/") == "" {
		return defaultBaseURL
	}

	return strings.TrimRight(c.BaseURL, "/")
}

// User — ответ getMe: идентичность бота по его токену.
type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

// APIError — ответ Bot API со статусом 4xx: повторять бессмысленно, запрос
// надо чинить, а не ретраить через другой прокси.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("telegram api: HTTP %d: %s", e.StatusCode, e.Body)
}

type Client struct {
	cfg    Config
	logger *slog.Logger
}

func NewClient(cfg Config, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}

	return &Client{cfg: cfg, logger: logger}
}

// Enabled — есть ли токен для вызовов.
func (c *Client) Enabled() bool {
	return c.cfg.Token != ""
}

func (c *Client) GetMe(ctx context.Context) (User, error) {
	raw, err := c.Call(ctx, "getMe", nil)
	if err != nil {
		return User{}, err
	}

	var user User
	if err := json.Unmarshal(raw, &user); err != nil {
		return User{}, fmt.Errorf("decode getMe response: %w", err)
	}

	return user, nil
}

func (c *Client) Call(ctx context.Context, method string, params url.Values) (json.RawMessage, error) {
	if !c.Enabled() {
		return nil, ErrNoToken
	}

	endpoint := fmt.Sprintf("%s/bot%s/%s", c.cfg.baseURL(), c.cfg.Token, method)
	body, err := c.attempt(ctx, method, func(ctx context.Context, httpClient *http.Client) (json.RawMessage, error) {
		return c.do(ctx, httpClient, endpoint, params)
	})
	if err != nil {
		return nil, err
	}

	return body, nil
}

// GetJSON выполняет GET к произвольному URL с той же логикой ретраев и
// ротации прокси, что и вызовы Bot API. Токен не требуется и не передаётся:
// нужен для публичных endpoint'ов OIDC (JWKS). Ответ возвращается как есть,
// без разбора Bot API-конверта.
func (c *Client) GetJSON(ctx context.Context, rawURL string) (json.RawMessage, error) {
	return c.attempt(ctx, "GET", func(ctx context.Context, httpClient *http.Client) (json.RawMessage, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()

		respBody, err := readAll(resp.Body)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), errBodyLimit))
		}

		if resp.StatusCode != http.StatusOK {
			return nil, &APIError{StatusCode: resp.StatusCode, Body: truncate(string(respBody), errBodyLimit)}
		}

		return json.RawMessage(respBody), nil
	})
}

// attempt гоняет one Attempt по кругу прокси: ретраит транспортные сбои и
// 429/5xx, на 4xx сдаётся сразу.
func (c *Client) attempt(
	ctx context.Context,
	method string,
	one func(ctx context.Context, httpClient *http.Client) (json.RawMessage, error),
) (json.RawMessage, error) {
	attempts := c.cfg.attempts()

	var lastErr error
	for i := range attempts {
		if i > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryBackoff):
			}
		}

		httpClient := &http.Client{
			Transport: c.transportFor(i),
			Timeout:   c.cfg.timeout(),
		}

		body, err := one(ctx, httpClient)
		var apiErr *APIError
		if err == nil {
			if i > 0 {
				c.logger.Info("telegram api attempt succeeded", slog.String("method", method), slog.Int("attempt", i+1))
			}

			return body, nil
		}

		lastErr = err
		if errors.As(err, &apiErr) {
			return nil, err
		}

		proxy := "direct"
		if len(c.cfg.Proxies) > 0 {
			proxy = c.cfg.Proxies[i%len(c.cfg.Proxies)].Redacted()
		}

		c.logger.Warn(
			"telegram api attempt failed",
			slog.String("method", method),
			slog.Int("attempt", i+1),
			slog.Int("max_attempts", attempts),
			slog.String("proxy", proxy),
			slog.Any("error", err),
		)
	}

	return nil, fmt.Errorf("telegram api: %d attempts failed, last error: %w", attempts, lastErr)
}

// transportFor собирает транспорт для i-й попытки: прокси по кругу, без
// прокси — прямой. Используются только http(s)-прокси: для них Transport
// сам поднимает CONNECT-туннель.
func (c *Client) transportFor(i int) *http.Transport {
	transport := &http.Transport{}
	if len(c.cfg.Proxies) == 0 {
		return transport
	}

	proxy := c.cfg.Proxies[i%len(c.cfg.Proxies)]

	transport.Proxy = http.ProxyURL(proxy)

	return transport
}

// do выполняет одну попытку POST Bot API. *APIError — ответ исправен, ретрай
// бессмысленен; любая другая ошибка — транспортный сбой или 429/5xx, попытку
// можно повторить через другой прокси.
func (c *Client) do(
	ctx context.Context,
	httpClient *http.Client,
	endpoint string,
	params url.Values,
) (json.RawMessage, error) {
	var body *strings.Reader
	if params == nil {
		body = strings.NewReader("")
	} else {
		body = strings.NewReader(params.Encode())
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), errBodyLimit))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: truncate(string(respBody), errBodyLimit)}
	}

	var envelope struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
	}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return nil, fmt.Errorf("decode telegram response: %w", err)
	}
	if !envelope.OK {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: envelope.Description}
	}

	return envelope.Result, nil
}
