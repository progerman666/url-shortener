package service

import (
	"context"
	"crypto/rand"
	"errors"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/progerman666/url-shortener/internal/repository"
)

var (
	ErrInvalidURL = errors.New("invalid url")
	ErrBadScheme  = errors.New("only http/https urls are allowed")
)

const (
	codeLength = 7
	alphabet   = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	maxRetries = 5
)

// LinkRepository — интерфейс того, что нужно сервису от хранилища.
// *repository.LinkRepository удовлетворяет ему неявно.
type LinkRepository interface {
	Create(ctx context.Context, code, url string, expiresAt *time.Time) (*repository.Link, error)
	GetByCode(ctx context.Context, code string) (*repository.Link, error)
	Stats(ctx context.Context, code string) (*repository.Stats, error)
}

type LinkService struct {
	repo LinkRepository
}

func NewLinkService(repo LinkRepository) *LinkService {
	return &LinkService{repo: repo}
}

func (s *LinkService) Create(ctx context.Context, rawURL string, ttl time.Duration) (*repository.Link, error) {
	if err := validateURL(rawURL); err != nil {
		return nil, err
	}

	var expiresAt *time.Time
	if ttl > 0 {
		t := time.Now().Add(ttl)
		expiresAt = &t
	}

	for i := 0; i < maxRetries; i++ {
		code, err := generateCode(codeLength)
		if err != nil {
			return nil, err
		}
		link, err := s.repo.Create(ctx, code, rawURL, expiresAt)
		if err == nil {
			return link, nil
		}
		if !isUniqueViolation(err) {
			return nil, err
		}
	}
	return nil, errors.New("failed to generate unique code")
}

func (s *LinkService) Resolve(ctx context.Context, code string) (*repository.Link, error) {
	return s.repo.GetByCode(ctx, code)
}

func (s *LinkService) Stats(ctx context.Context, code string) (*repository.Stats, error) {
	return s.repo.Stats(ctx, code)
}

func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return ErrInvalidURL
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrBadScheme
	}
	if u.Host == "" {
		return ErrInvalidURL
	}
	return nil
}

func generateCode(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i, v := range b {
		b[i] = alphabet[int(v)%len(alphabet)]
	}
	return string(b), nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}