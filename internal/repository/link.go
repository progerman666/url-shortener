package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("link not found")
	ErrExpired  = errors.New("link expired")
)

type Link struct {
	ID        int64
	Code      string
	URL       string
	CreatedAt time.Time
	ExpiresAt *time.Time
}

type Stats struct {
	Clicks        int64      `json:"clicks"`
	LastClickedAt *time.Time `json:"last_clicked_at,omitempty"`
}

type LinkRepository struct {
	db *sql.DB
}

func NewLinkRepository(db *sql.DB) *LinkRepository {
	return &LinkRepository{db: db}
}

func (r *LinkRepository) Create(ctx context.Context, code, url string, expiresAt *time.Time) (*Link, error) {
	row := r.db.QueryRowContext(ctx,
		`INSERT INTO links (code, url, expires_at) VALUES ($1, $2, $3)
		 RETURNING id, code, url, created_at, expires_at`,
		code, url, expiresAt,
	)
	var l Link
	if err := row.Scan(&l.ID, &l.Code, &l.URL, &l.CreatedAt, &l.ExpiresAt); err != nil {
		return nil, err
	}
	return &l, nil
}

func (r *LinkRepository) GetByCode(ctx context.Context, code string) (*Link, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, code, url, created_at, expires_at FROM links WHERE code = $1`,
		code,
	)
	var l Link
	if err := row.Scan(&l.ID, &l.Code, &l.URL, &l.CreatedAt, &l.ExpiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if l.ExpiresAt != nil && l.ExpiresAt.Before(time.Now()) {
		return nil, ErrExpired
	}
	return &l, nil
}

func (r *LinkRepository) InsertClick(ctx context.Context, linkID int64, ua, ip string) error {
	var ipArg any
	if ip != "" {
		ipArg = ip
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO clicks (link_id, user_agent, ip) VALUES ($1, $2, $3)`,
		linkID, ua, ipArg,
	)
	return err
}

func (r *LinkRepository) Stats(ctx context.Context, code string) (*Stats, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT l.id,
		       COALESCE((SELECT count(*) FROM clicks c WHERE c.link_id = l.id), 0),
		       (SELECT max(clicked_at) FROM clicks c WHERE c.link_id = l.id)
		FROM links l WHERE l.code = $1`, code)

	var linkID int64
	var s Stats
	if err := row.Scan(&linkID, &s.Clicks, &s.LastClickedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}