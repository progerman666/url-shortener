CREATE TABLE IF NOT EXISTS links (
    id          BIGSERIAL PRIMARY KEY,
    code        TEXT NOT NULL UNIQUE,
    url         TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_links_expires_at ON links(expires_at)
    WHERE expires_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS clicks (
    id         BIGSERIAL PRIMARY KEY,
    link_id    BIGINT NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    clicked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    user_agent TEXT,
    ip         INET
);

CREATE INDEX IF NOT EXISTS idx_clicks_link_id ON clicks(link_id);