package battlepass_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func BPMigrateV2(ctx context.Context, pool *pgxpool.Pool) error {
	sql := `
CREATE TABLE IF NOT EXISTS bp_seasons(
	id BIGSERIAL PRIMARY KEY,
	label VARCHAR(64) NOT NULL,
	starts_at TIMESTAMPTZ NOT NULL,
	ends_at TIMESTAMPTZ NOT NULL,
	is_active BOOLEAN NOT NULL DEFAULT true,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_bp_seasons_one_active ON bp_seasons(is_active) WHERE is_active;

CREATE TABLE IF NOT EXISTS bp_profiles(
	user_id INT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
	season_id BIGINT NOT NULL REFERENCES bp_seasons(id) ON DELETE CASCADE,
	xp BIGINT NOT NULL DEFAULT 0,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS bp_passes(
	user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	season_id BIGINT NOT NULL REFERENCES bp_seasons(id) ON DELETE CASCADE,
	purchased_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY(user_id, season_id)
);

CREATE TABLE IF NOT EXISTS bp_claims(
	user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	season_id BIGINT NOT NULL REFERENCES bp_seasons(id) ON DELETE CASCADE,
	level SMALLINT NOT NULL,
	track VARCHAR(8) NOT NULL,
	claimed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY(user_id, season_id, level, track)
);

CREATE TABLE IF NOT EXISTS bp_quests(
	id BIGSERIAL PRIMARY KEY,
	user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	season_id BIGINT NOT NULL REFERENCES bp_seasons(id) ON DELETE CASCADE,
	category VARCHAR(8) NOT NULL,
	period_key VARCHAR(24) NOT NULL,
	period_start TIMESTAMPTZ NOT NULL,
	period_end TIMESTAMPTZ NOT NULL,
	slot SMALLINT NOT NULL,
	template_id VARCHAR(64) NOT NULL,
	title VARCHAR(160) NOT NULL,
	metric VARCHAR(48) NOT NULL,
	mode VARCHAR(8) NOT NULL,
	target DOUBLE PRECISION NOT NULL,
	progress DOUBLE PRECISION NOT NULL DEFAULT 0,
	unit VARCHAR(16) NOT NULL DEFAULT 'count',
	xp INT NOT NULL,
	paid BOOLEAN NOT NULL DEFAULT false,
	completed_at TIMESTAMPTZ,
	claimed_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	UNIQUE(user_id, season_id, category, period_key, slot)
);
CREATE INDEX IF NOT EXISTS idx_bp_quests_lookup ON bp_quests(user_id, season_id, category, period_key);
CREATE INDEX IF NOT EXISTS idx_bp_quests_metric ON bp_quests(user_id, season_id, metric) WHERE claimed_at IS NULL;

CREATE TABLE IF NOT EXISTS bp_events(
	id BIGSERIAL PRIMARY KEY,
	user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	season_id BIGINT NOT NULL REFERENCES bp_seasons(id) ON DELETE CASCADE,
	event_key VARCHAR(160) NOT NULL,
	metric VARCHAR(48) NOT NULL,
	amount DOUBLE PRECISION NOT NULL,
	occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	UNIQUE(user_id, season_id, event_key)
);
CREATE INDEX IF NOT EXISTS idx_bp_events_metric ON bp_events(user_id, season_id, metric, occurred_at);

CREATE TABLE IF NOT EXISTS user_statuses(
	id SERIAL PRIMARY KEY,
	user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	status VARCHAR(24) NOT NULL,
	granted_at TIMESTAMP NOT NULL DEFAULT now(),
	UNIQUE(user_id, status)
);

CREATE TABLE IF NOT EXISTS bp_case_tokens(
	id BIGSERIAL PRIMARY KEY,
	user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	season_id BIGINT NOT NULL REFERENCES bp_seasons(id) ON DELETE CASCADE,
	rarity VARCHAR(16) NOT NULL,
	source VARCHAR(16) NOT NULL DEFAULT 'level',
	source_level SMALLINT NOT NULL DEFAULT 0,
	source_track VARCHAR(8) NOT NULL DEFAULT 'free',
	reward JSONB NOT NULL DEFAULT '{}'::jsonb,
	roll JSONB,
	opened_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_bp_case_tokens_pending ON bp_case_tokens(user_id, season_id) WHERE opened_at IS NULL;

DO $$
BEGIN
	IF to_regclass('public.referrals') IS NOT NULL THEN
		ALTER TABLE referrals ADD COLUMN IF NOT EXISTS ref_xp INT NOT NULL DEFAULT 0;
	END IF;
END $$;
`
	_, err := pool.Exec(ctx, sql)
	return err
}
