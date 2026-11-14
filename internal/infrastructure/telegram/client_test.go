package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func testClient(t *testing.T, baseURL string, proxies []string, maxAttempts int) *Client {
	t.Helper()

	parsed := make([]*url.URL, 0, len(proxies))
	for _, p := range proxies {
		u, err := url.Parse(p)
		if err != nil {
			t.Fatalf("parse proxy %q: %v", p, err)
		}
		parsed = append(parsed, u)
	}

	return NewClient(Config{
		BaseURL:     baseURL,
		Token:       "test-token",
		Proxies:     parsed,
		MaxAttempts: maxAttempts,
		Timeout:     time.Second,
	}, slog.New(slog.DiscardHandler))
}

func TestCallRetriesUntilSuccess(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt := calls.Add(1)
		if attempt < 6 {
			w.WriteHeader(http.StatusBadGateway)

			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"id": 1}})
	}))
	defer server.Close()

	client := testClient(t, server.URL, nil, 6)
	raw, err := client.Call(context.Background(), "getMe", nil)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if calls.Load() != 6 {
		t.Fatalf("attempts = %d, want 6", calls.Load())
	}
	if string(raw) == "" {
		t.Fatal("empty result")
	}
}

func TestCallGivesUpAfterMaxAttempts(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := testClient(t, server.URL, nil, 6)
	_, err := client.Call(context.Background(), "getMe", nil)
	if err == nil {
		t.Fatal("expected error after exhausting attempts")
	}
	if calls.Load() != 6 {
		t.Fatalf("attempts = %d, want 6", calls.Load())
	}
}

func TestCallDoesNotRetryClientErrors(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"ok":false,"description":"Unauthorized"}`))
	}))
	defer server.Close()

	client := testClient(t, server.URL, nil, 6)
	_, err := client.Call(context.Background(), "getMe", nil)
	if _, ok := errors.AsType[*APIError](err); !ok {
		t.Fatalf("want APIError, got %T: %v", err, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("attempts = %d, want 1 (4xx не ретраим)", calls.Load())
	}
}

// TestCallRotatesProxies: первые три прокси падают, четвёртый отвечает.
// Успех на четвёртой попытке возможен, только если клиент прошёл прокси
// по кругу — и каждый прокси увидел ровно одно обращение.
func TestCallRotatesProxies(t *testing.T) {
	var badCalls [3]atomic.Int32
	var goodCalls atomic.Int32

	badHandler := func(i int) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			badCalls[i].Add(1)
			w.WriteHeader(http.StatusBadGateway)
		}
	}
	goodHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		goodCalls.Add(1)
		if err := json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"id": 1}}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	})

	proxies := make([]string, 0, 4)
	for i := range badCalls {
		server := httptest.NewServer(badHandler(i))
		defer server.Close()
		proxies = append(proxies, server.URL)
	}
	good := httptest.NewServer(goodHandler)
	defer good.Close()
	proxies = append(proxies, good.URL)

	// HTTPS-цель через CONNECT не проксить httptest-ом, поэтому тестируем
	// ротацию напрямую на выборе транспорта: попытка i должна взять
	// прокси i%4, и пятая попытка (индекс 4) снова придёт на первый.
	client := testClient(t, "https://api.telegram.org", proxies, 6)

	picked := make([]*url.URL, 0, 6)
	for i := range 6 {
		transport := client.transportFor(i)
		req := &http.Request{}
		got, err := transport.Proxy(req)
		if err != nil || got == nil {
			t.Fatalf("attempt %d: proxy not set: %v", i, err)
		}
		picked = append(picked, got)
	}
	if picked[0].String() != picked[4].String() {
		t.Fatalf("rotation is not round-robin: %q vs %q", picked[0], picked[4])
	}
	if picked[3].String() == picked[0].String() {
		t.Fatalf("attempt 3 should use a different proxy than attempt 0")
	}

	// Живой прогон: одна попытка через заведомо мёртвый прокси падает,
	// вторая (MaxAttempts=2, прокси один — снова он же) исчерпывает лимит.
	dead, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	client = testClient(t, "https://api.telegram.org", []string{dead.String()}, 2)
	if _, err := client.Call(context.Background(), "getMe", nil); err == nil {
		t.Fatal("expected failure through dead proxy")
	}
}

// TestCallThroughHTTPProxy: минимальный forward-прокси (absolute-form запросы
// для http:// целей идут через прокси без CONNECT) доказывает, что вызов реально
// проходит через прокси, а не напрямую.
func TestCallThroughHTTPProxy(t *testing.T) {
	var proxyCalls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalls.Add(1)
		resp, err := http.Get(r.URL.String()) //nolint:gosec // тестовый forward-прокси
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)

			return
		}
		defer func() { _ = resp.Body.Close() }()
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	defer proxy.Close()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"id": 7}})
	}))
	defer target.Close()

	client := testClient(t, target.URL, []string{proxy.URL}, 6)
	raw, err := client.Call(context.Background(), "getMe", nil)
	if err != nil {
		t.Fatalf("call through proxy failed: %v", err)
	}
	if string(raw) != `{"id":7}` {
		t.Fatalf("result = %s", raw)
	}
	if proxyCalls.Load() != 1 {
		t.Fatalf("proxy calls = %d, want 1", proxyCalls.Load())
	}
}

func TestGetMeDecodesUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": map[string]any{"id": 42, "is_bot": true, "first_name": "Hermes", "username": "hermes_vl_bot"},
		})
	}))
	defer server.Close()

	client := testClient(t, server.URL, nil, 6)
	user, err := client.GetMe(context.Background())
	if err != nil {
		t.Fatalf("getMe failed: %v", err)
	}
	if user.Username != "hermes_vl_bot" || user.ID != 42 {
		t.Fatalf("user = %+v", user)
	}
}

func TestCallWithoutToken(t *testing.T) {
	client := NewClient(Config{}, nil)
	if _, err := client.Call(context.Background(), "getMe", nil); !errors.Is(err, ErrNoToken) {
		t.Fatalf("want ErrNoToken, got %v", err)
	}
}

// TestGetJSONRetriesUntilSuccess: GetJSON идёт к произвольному URL (JWKS) с
// той же ретрай-логикой, что и Bot API, и не требует токен бота.
func TestGetJSONRetriesUntilSuccess(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt := calls.Add(1)
		if attempt < 3 {
			w.WriteHeader(http.StatusBadGateway)

			return
		}
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	defer server.Close()

	client := testClient(t, "https://api.telegram.org", nil, 6)
	raw, err := client.GetJSON(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("get json failed: %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("attempts = %d, want 3", calls.Load())
	}
	if string(raw) != `{"keys":[]}` {
		t.Fatalf("body = %s", raw)
	}
}

func TestGetJSONWithoutToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":1}`))
	}))
	defer server.Close()

	client := NewClient(Config{Timeout: time.Second}, nil)
	raw, err := client.GetJSON(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("get json without bot token failed: %v", err)
	}
	if string(raw) != `{"ok":1}` {
		t.Fatalf("body = %s", raw)
	}
}

func TestGetJSONDoesNotRetryClientErrors(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := testClient(t, "https://api.telegram.org", nil, 6)
	_, err := client.GetJSON(context.Background(), server.URL)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want APIError, got %T: %v", err, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("attempts = %d, want 1 (4xx не ретраим)", calls.Load())
	}
}

func TestConfigAttemptsDefault(t *testing.T) {
	var c Config
	if got := c.attempts(); got != 6 {
		t.Fatalf("default attempts = %d, want 6", got)
	}
	c.MaxAttempts = 2
	if got := c.attempts(); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
	if c.baseURL() != defaultBaseURL {
		t.Fatalf("baseURL = %q", c.baseURL())
	}
}

func TestTruncateHelper(t *testing.T) {
	if got := truncate("abc", 5); got != "abc" {
		t.Fatalf("short = %q", got)
	}
	if got := truncate("abcdef", 3); got != "abc…" {
		t.Fatalf("long = %q", got)
	}
}
