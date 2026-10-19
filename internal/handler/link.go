package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/progerman666/url-shortener/internal/repository"
	"github.com/progerman666/url-shortener/internal/service"
	"github.com/progerman666/url-shortener/internal/worker"
)

// LinkService — интерфейс того, что нужно хендлеру от сервиса.
type LinkService interface {
	Create(ctx context.Context, rawURL string, ttl time.Duration) (*repository.Link, error)
	Resolve(ctx context.Context, code string) (*repository.Link, error)
	Stats(ctx context.Context, code string) (*repository.Stats, error)
}

// ClickEnqueuer — интерфейс того, что нужно хендлеру от воркера.
type ClickEnqueuer interface {
	Enqueue(e worker.ClickEvent) bool
}

type LinkHandler struct {
	svc    LinkService
	worker ClickEnqueuer
	prefix string
}

func NewLinkHandler(svc LinkService, w ClickEnqueuer, prefix string) *LinkHandler {
	return &LinkHandler{svc: svc, worker: w, prefix: prefix}
}

type createLinkRequest struct {
	URL            string `json:"url"`
	ExpiresInHours int    `json:"expires_in_hours,omitempty"`
}

type createLinkResponse struct {
	Code      string     `json:"code"`
	ShortURL  string     `json:"short_url"`
	URL       string     `json:"url"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func (h *LinkHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	ttl := time.Duration(req.ExpiresInHours) * time.Hour
	link, err := h.svc.Create(r.Context(), req.URL, ttl)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidURL), errors.Is(err, service.ErrBadScheme):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	writeJSON(w, http.StatusCreated, createLinkResponse{
		Code:      link.Code,
		ShortURL:  h.prefix + "/" + link.Code,
		URL:       link.URL,
		ExpiresAt: link.ExpiresAt,
	})
}

func (h *LinkHandler) Redirect(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	link, err := h.svc.Resolve(r.Context(), code)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound):
			writeError(w, http.StatusNotFound, "link not found")
		case errors.Is(err, repository.ErrExpired):
			writeError(w, http.StatusGone, "link expired")
		default:
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	_ = h.worker.Enqueue(worker.ClickEvent{
		LinkID:    link.ID,
		UserAgent: r.UserAgent(),
		IP:        clientIP(r),
	})

	http.Redirect(w, r, link.URL, http.StatusFound)
}

func (h *LinkHandler) Stats(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	stats, err := h.svc.Stats(r.Context(), code)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "link not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}