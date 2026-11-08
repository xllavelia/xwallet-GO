package profile_sql

import (
	"context"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Доступ вычисляется из старых статусов, чтобы учитывать выдачи после запуска.
const schemaSQL = `
CREATE TABLE IF NOT EXISTS profile_cosmetic_catalog (
 id TEXT PRIMARY KEY,
 kind TEXT NOT NULL CHECK (kind IN ('theme','emblem','pattern')),
 name TEXT NOT NULL,
 rarity TEXT NOT NULL,
 source TEXT NOT NULL CHECK (source IN ('starter','battlepass','case','shop','admin')),
 price INTEGER NOT NULL DEFAULT 0 CHECK (price >= 0),
 level SMALLINT CHECK (level BETWEEN 1 AND 50),
 track TEXT CHECK (track IN ('free','pass')),
 admin_only BOOLEAN NOT NULL DEFAULT false,
 CHECK (id LIKE kind || '-%'),
 CHECK ((source = 'admin') = admin_only),
 CHECK (NOT admin_only OR price = 0)
);
CREATE TABLE IF NOT EXISTS profile_legacy_cosmetics (
 status TEXT PRIMARY KEY,
 cosmetic_id TEXT NOT NULL REFERENCES profile_cosmetic_catalog(id)
);
CREATE TABLE IF NOT EXISTS user_profiles (
 user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 bio TEXT NOT NULL DEFAULT '' CHECK (char_length(bio) <= 160 AND bio !~ '[[:cntrl:]]'),
 theme_id TEXT NOT NULL DEFAULT 'theme-tide' REFERENCES profile_cosmetic_catalog(id) CHECK (theme_id LIKE 'theme-%'),
 emblem_id TEXT NOT NULL DEFAULT 'emblem-none' REFERENCES profile_cosmetic_catalog(id) CHECK (emblem_id LIKE 'emblem-%'),
 pattern_id TEXT NOT NULL DEFAULT 'pattern-orbit' REFERENCES profile_cosmetic_catalog(id) CHECK (pattern_id LIKE 'pattern-%'),
 show_balance BOOLEAN NOT NULL DEFAULT true,
 avatar BYTEA CHECK (octet_length(avatar) <= 98304),
 avatar_thumb BYTEA CHECK (octet_length(avatar_thumb) <= 12288),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK ((avatar IS NULL) = (avatar_thumb IS NULL))
);
CREATE TABLE IF NOT EXISTS user_cosmetics (
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 cosmetic_id TEXT NOT NULL REFERENCES profile_cosmetic_catalog(id),
 source TEXT NOT NULL CHECK (char_length(source) BETWEEN 1 AND 64 AND source !~ '[[:cntrl:]]'),
 granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY (user_id, cosmetic_id),
 CHECK (cosmetic_id NOT IN ('theme-xlavelia','theme-wave','emblem-sigil','pattern-monogram'))
);
CREATE TABLE IF NOT EXISTS community_friendships (
 user_low INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 user_high INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 requester_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 state TEXT NOT NULL CHECK (state IN ('pending','friend')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY (user_low, user_high),
 CHECK (user_low < user_high),
 CHECK (requester_id IN (user_low, user_high))
);
CREATE INDEX IF NOT EXISTS community_friendships_high ON community_friendships(user_high, state);
CREATE INDEX IF NOT EXISTS community_friendships_low ON community_friendships(user_low, state);
CREATE INDEX IF NOT EXISTS user_profiles_visible ON user_profiles(user_id) WHERE show_balance;
CREATE TABLE IF NOT EXISTS community_news (
 id TEXT PRIMARY KEY CHECK (char_length(id) BETWEEN 1 AND 64),
 title TEXT NOT NULL CHECK (char_length(title) BETWEEN 1 AND 120 AND title !~ '[[:cntrl:]<>]'),
 summary TEXT NOT NULL CHECK (char_length(summary) BETWEEN 1 AND 320 AND summary !~ '[[:cntrl:]<>]'),
 body TEXT NOT NULL CHECK (char_length(body) BETWEEN 1 AND 8000 AND body !~ '[<>]'),
 category TEXT NOT NULL CHECK (char_length(category) BETWEEN 1 AND 32 AND category !~ '[[:cntrl:]<>]'),
 art TEXT NOT NULL CHECK (art IN ('launch','season','community','market')),
 pinned BOOLEAN NOT NULL DEFAULT false,
 is_published BOOLEAN NOT NULL DEFAULT false,
 published_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK ((title || ' ' || summary || ' ' || body || ' ' || category) !~* '(https?://|www[.]|mailto:)')
);
CREATE INDEX IF NOT EXISTS community_news_published ON community_news(pinned DESC, published_at DESC, id) WHERE is_published;
CREATE TABLE IF NOT EXISTS profile_migrations (
 name TEXT PRIMARY KEY,
 applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`

const seedNewsSQL = `INSERT INTO community_news
(id,title,summary,body,category,art,pinned,is_published,published_at) VALUES
('welcome','Welcome to your profile','Make your player profile your own.',
 'Your player ID stays the same. You can edit your username, bio, avatar and balance visibility in your profile.',
 'Launch','launch',true,true,now()),
('cosmetics','Cosmetics and ownership','Choose from your available themes, emblems and patterns.',
 'Starter cosmetics are available to everyone. Other cosmetics require ownership. Existing legacy statuses unlock their corresponding cosmetics. Shop themes cost LAVX.',
 'Profiles','season',false,true,now()),
('community','Community connections','Find players and manage mutual friendships.',
 'Search by player ID or username and send a friend request. Friendships require acceptance. The leaderboard lists visible USD wallet balances and excludes administrator and banned accounts.',
 'Community','community',false,true,now())
ON CONFLICT (id) DO NOTHING`

func MigrateSchema(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Общая блокировка защищает миграцию и однократное наполнение при параллельных запусках.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(1735390031)`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, schemaSQL); err != nil {
		return err
	}
	for _, c := range Catalog() {
		var level any
		var track any
		if c.Level > 0 {
			level, track = c.Level, c.Track
		}
		_, err = tx.Exec(ctx, `INSERT INTO profile_cosmetic_catalog
(id,kind,name,rarity,source,price,level,track,admin_only) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (id) DO UPDATE SET kind=EXCLUDED.kind,name=EXCLUDED.name,rarity=EXCLUDED.rarity,
source=EXCLUDED.source,price=EXCLUDED.price,level=EXCLUDED.level,track=EXCLUDED.track,admin_only=EXCLUDED.admin_only`,
			c.ID, c.Kind, c.Name, c.Rarity, c.Source, c.Price, level, track, c.AdminOnly)
		if err != nil {
			return err
		}
	}
	keys := make([]string, 0, len(legacyCosmetics))
	for status := range legacyCosmetics {
		keys = append(keys, status)
	}
	sort.Strings(keys)
	for _, status := range keys {
		if _, err = tx.Exec(ctx, `INSERT INTO profile_legacy_cosmetics(status,cosmetic_id) VALUES ($1,$2)
ON CONFLICT(status) DO UPDATE SET cosmetic_id=EXCLUDED.cosmetic_id`, status, LegacyCosmetic(status)); err != nil {
			return err
		}
	}
	tag, err := tx.Exec(ctx, `INSERT INTO profile_migrations(name) VALUES ('introductory-news-v1') ON CONFLICT DO NOTHING`)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		if _, err = tx.Exec(ctx, seedNewsSQL); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
