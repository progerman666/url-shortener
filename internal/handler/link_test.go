package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/progerman666/url-shortener/internal/handler"
	"github.com/progerman666/url-shortener/internal/repository"
	"github.com/progerman666/url-shortener/internal/service"
	"github.com/progerman666/url-shortener/internal/worker"
)

// ---------- mocks ----------

type mockService struct {
	createFn  func(ctx context.Context, rawURL string, ttl time.Duration) (*repository.Link, error)
	resolveFn func(ctx context.Context, code string) (*repository.Link, error)
	statsFn   func(ctx context.Context, code string) (*repository.Stats, error)
}

func (m *mockService) Create(ctx context.Context, rawURL string, ttl time.Duration) (*repository.Link, error) {
	return m.createFn(ctx, rawURL, ttl)
}
func (m *mockService) Resolve(ctx context.Context, code string) (*repository.Link, error) {
	return m.resolveFn(ctx, code)
}
func (m *mockService) Stats(ctx context.Context, code string) (*repository.Stats, error) {
	return m.statsFn(ctx, code)
}

type mockWorker struct {
	enqueued []worker.ClickEvent
	full     bool
}

func (m *mockWorker) Enqueue(e worker.ClickEvent) bool {
	if m.full {
		return false
	}
	m.enqueued = append(m.enqueued, e)
	return true
}

// ---------- helpers ----------

func newMux(h *handler.LinkHandler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /links", h.Create)
	mux.HandleFunc("GET /links/{code}/stats", h.Stats)
	mux.HandleFunc("GET /{code}", h.Redirect)
	return mux
}

// ---------- tests ----------

func TestCreate_Success(t *testing.T) {
	expires := time.Now().Add(24 * time.Hour)
	svc := &mockService{
		createFn: func(ctx context.Context, rawURL string, ttl time.Duration) (*repository.Link, error) {
			return &repository.Link{ID: 1, Code: "abc123", URL: rawURL, ExpiresAt: &expires}, nil
		},
	}
	h := handler.NewLinkHandler(svc, &mockWorker{}, "http://test.local")
	mux := newMux(h)

	body := bytes.NewBufferString(`{"url":"https://example.com","expires_in_hours":24}`)
	req := httptest.NewRequest("POST", "/links", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Code     string `json:"code"`
		ShortURL string `json:"short_url"`
		URL      string `json:"url"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp.Code != "abc123" {
		t.Errorf("code: want abc123, got %q", resp.Code)
	}
	if resp.ShortURL != "http://test.local/abc123" {
		t.Errorf("short_url: want http://test.local/abc123, got %q", resp.ShortURL)
	}
	if resp.URL != "https://example.com" {
		t.Errorf("url: want https://example.com, got %q", resp.URL)
	}
}

func TestCreate_InvalidURL(t *testing.T) {
	svc := &mockService{
		createFn: func(ctx context.Context, rawURL string, ttl time.Duration) (*repository.Link, error) {
			return nil, service.ErrInvalidURL
		},
	}
	h := handler.NewLinkHandler(svc, &mockWorker{}, "http://test.local")
	mux := newMux(h)

	req := httptest.NewRequest("POST", "/links", strings.NewReader(`{"url":"not-a-url"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	var resp map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] == "" {
		t.Errorf("want error field in body, got %v", resp)
	}
}

func TestCreate_BadJSON(t *testing.T) {
	svc := &mockService{
		createFn: func(ctx context.Context, rawURL string, ttl time.Duration) (*repository.Link, error) {
			t.Fatal("service.Create must not be called for bad JSON")
			return nil, nil
		},
	}
	h := handler.NewLinkHandler(svc, &mockWorker{}, "http://test.local")
	mux := newMux(h)

	req := httptest.NewRequest("POST", "/links", strings.NewReader(`not json`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestCreate_MissingURL(t *testing.T) {
	svc := &mockService{
		createFn: func(ctx context.Context, rawURL string, ttl time.Duration) (*repository.Link, error) {
			t.Fatal("service.Create must not be called when url is empty")
			return nil, nil
		},
	}
	h := handler.NewLinkHandler(svc, &mockWorker{}, "http://test.local")
	mux := newMux(h)

	req := httptest.NewRequest("POST", "/links", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestRedirect_Success(t *testing.T) {
	svc := &mockService{
		resolveFn: func(ctx context.Context, code string) (*repository.Link, error) {
			if code != "abc123" {
				t.Fatalf("unexpected code %q", code)
			}
			return &repository.Link{ID: 42, Code: "abc123", URL: "https://example.com"}, nil
		},
	}
	w := &mockWorker{}
	h := handler.NewLinkHandler(svc, w, "http://test.local")
	mux := newMux(h)

	req := httptest.NewRequest("GET", "/abc123", nil)
	req.Header.Set("User-Agent", "test-agent")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "https://example.com" {
		t.Errorf("Location: want https://example.com, got %q", loc)
	}
	if len(w.enqueued) != 1 {
		t.Fatalf("want 1 click enqueued, got %d", len(w.enqueued))
	}
	if w.enqueued[0].LinkID != 42 {
		t.Errorf("LinkID: want 42, got %d", w.enqueued[0].LinkID)
	}
	if w.enqueued[0].UserAgent != "test-agent" {
		t.Errorf("UserAgent: want test-agent, got %q", w.enqueued[0].UserAgent)
	}
}

func TestRedirect_NotFound(t *testing.T) {
	svc := &mockService{
		resolveFn: func(ctx context.Context, code string) (*repository.Link, error) {
			return nil, repository.ErrNotFound
		},
	}
	h := handler.NewLinkHandler(svc, &mockWorker{}, "http://test.local")
	mux := newMux(h)

	req := httptest.NewRequest("GET", "/missing", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestRedirect_Expired(t *testing.T) {
	svc := &mockService{
		resolveFn: func(ctx context.Context, code string) (*repository.Link, error) {
			return nil, repository.ErrExpired
		},
	}
	h := handler.NewLinkHandler(svc, &mockWorker{}, "http://test.local")
	mux := newMux(h)

	req := httptest.NewRequest("GET", "/expired", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusGone {
		t.Fatalf("want 410, got %d", rec.Code)
	}
}

func TestStats_Success(t *testing.T) {
	now := time.Now()
	svc := &mockService{
		statsFn: func(ctx context.Context, code string) (*repository.Stats, error) {
			return &repository.Stats{Clicks: 7, LastClickedAt: &now}, nil
		},
	}
	h := handler.NewLinkHandler(svc, &mockWorker{}, "http://test.local")
	mux := newMux(h)

	req := httptest.NewRequest("GET", "/links/abc123/stats", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var s repository.Stats
	_ = json.NewDecoder(rec.Body).Decode(&s)
	if s.Clicks != 7 {
		t.Errorf("clicks: want 7, got %d", s.Clicks)
	}
}

func TestStats_NotFound(t *testing.T) {
	svc := &mockService{
		statsFn: func(ctx context.Context, code string) (*repository.Stats, error) {
			return nil, repository.ErrNotFound
		},
	}
	h := handler.NewLinkHandler(svc, &mockWorker{}, "http://test.local")
	mux := newMux(h)

	req := httptest.NewRequest("GET", "/links/missing/stats", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}