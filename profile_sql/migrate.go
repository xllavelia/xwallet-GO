package profile_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Каталог теперь хранит только дизайны карточек; старые темы и эмблемы остаются в таблице, но не используются.
const schemaSQL = `
CREATE TABLE IF NOT EXISTS profile_cosmetic_catalog (
 id TEXT PRIMARY KEY,
 kind TEXT NOT NULL CHECK (kind IN ('card')),
 name TEXT NOT NULL,
 rarity TEXT NOT NULL,
 source TEXT NOT NULL CHECK (source IN ('starter','battlepass','case','shop','admin')),
 price INTEGER NOT NULL DEFAULT 0 CHECK (price >= 0),
 level SMALLINT CHECK (level BETWEEN 1 AND 50),
 track TEXT CHECK (track IN ('free','pass')),
 admin_only BOOLEAN NOT NULL DEFAULT false,
 colors JSONB,
 CHECK (id LIKE 'card-%'),
 CHECK ((source = 'admin') = admin_only),
 CHECK (NOT admin_only OR price = 0)
);
-- Продакшн-таблица создавалась до дизайнов карточек: добавляем colors и разрешаем kind='card'.
ALTER TABLE profile_cosmetic_catalog ADD COLUMN IF NOT EXISTS colors JSONB;
DO $$
DECLARE con record;
BEGIN
 FOR con IN
  SELECT conname FROM pg_constraint
  WHERE conrelid = 'profile_cosmetic_catalog'::regclass
    AND contype = 'c'
    AND pg_get_constraintdef(oid) LIKE '%theme%'
    AND pg_get_constraintdef(oid) LIKE '%kind%'
 LOOP
  EXECUTE format('ALTER TABLE profile_cosmetic_catalog DROP CONSTRAINT %I', con.conname);
 END LOOP;
END $$;
DO $$
BEGIN
 IF NOT EXISTS (
  SELECT 1 FROM pg_constraint
  WHERE conrelid = 'profile_cosmetic_catalog'::regclass
    AND conname = 'profile_cosmetic_catalog_kind_check'
    AND pg_get_constraintdef(oid) LIKE '%card%'
 ) THEN
  ALTER TABLE profile_cosmetic_catalog ADD CONSTRAINT profile_cosmetic_catalog_kind_check CHECK (kind IN ('card')) NOT VALID;
 END IF;
END $$;
CREATE TABLE IF NOT EXISTS user_profiles (
 user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 bio TEXT NOT NULL DEFAULT '' CHECK (char_length(bio) <= 160 AND bio !~ '[[:cntrl:]]'),
 theme_id TEXT NOT NULL DEFAULT 'theme-tide',
 emblem_id TEXT NOT NULL DEFAULT 'emblem-none',
 pattern_id TEXT NOT NULL DEFAULT 'pattern-orbit',
 show_balance BOOLEAN NOT NULL DEFAULT true,
 avatar BYTEA CHECK (octet_length(avatar) <= 98304),
 avatar_thumb BYTEA CHECK (octet_length(avatar_thumb) <= 12288),
 banner BYTEA CHECK (octet_length(banner) <= 204800),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK ((avatar IS NULL) = (avatar_thumb IS NULL))
);
ALTER TABLE user_profiles ADD COLUMN IF NOT EXISTS banner BYTEA CHECK (octet_length(banner) <= 204800);
-- Старые внешние ключи тем/эмблем мешают обновлять профиль после перехода на дизайны карточек.
ALTER TABLE user_profiles DROP CONSTRAINT IF EXISTS user_profiles_theme_id_fkey;
ALTER TABLE user_profiles DROP CONSTRAINT IF EXISTS user_profiles_emblem_id_fkey;
ALTER TABLE user_profiles DROP CONSTRAINT IF EXISTS user_profiles_pattern_id_fkey;
ALTER TABLE user_cosmetics DROP CONSTRAINT IF EXISTS user_cosmetics_cosmetic_id_fkey;
CREATE TABLE IF NOT EXISTS user_cosmetics (
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 cosmetic_id TEXT NOT NULL REFERENCES profile_cosmetic_catalog(id),
 source TEXT NOT NULL CHECK (char_length(source) BETWEEN 1 AND 64 AND source !~ '[[:cntrl:]]'),
 granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY (user_id, cosmetic_id),
 CHECK (cosmetic_id NOT IN ('card-xlavelia','card-wave'))
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
CREATE INDEX IF NOT EXISTS community_friendships_high ON community_friendships (user_high, state);
CREATE INDEX IF NOT EXISTS community_friendships_low ON community_friendships (user_low, state);
CREATE INDEX IF NOT EXISTS user_profiles_visible ON user_profiles (user_id) WHERE show_balance;
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
CREATE INDEX IF NOT EXISTS community_news_published ON community_news (pinned DESC, published_at DESC, id) WHERE is_published;
CREATE TABLE IF NOT EXISTS profile_migrations (
 name TEXT PRIMARY KEY,
 applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`

const seedNewsSQL = `INSERT INTO community_news
(id,title,summary,body,category,art,pinned,is_published,published_at) VALUES
('welcome','Welcome to your profile','Make your player profile your own.',
 'Your player ID stays the same. You can edit your username, bio, avatar, banner and balance visibility in your profile.',
 'Launch','launch',true,true,now()),
('cosmetics','Card designs and ownership','Collect designs and make your cards your own.',
 'Card designs come from Battle Pass, cases and the LAVX shop. Open an account and pick any design you own, or create a fully custom card.',
 'Profiles','season',false,true,now()),
('community','Community connections','Find players and manage mutual friendships.',
 'Search by player ID or username and send a friend request. Friendships require acceptance. The leaderboard lists visible USD wallet balances and excludes administrator and banned accounts.',
 'Community','community',false,true,now())
ON CONFLICT (id) DO UPDATE SET title=EXCLUDED.title,summary=EXCLUDED.summary,body=EXCLUDED.body`

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
		var colors any
		if c.Level > 0 {
			level, track = c.Level, c.Track
		}
		if len(c.Colors) > 0 {
			colors = c.Colors
		}
		_, err = tx.Exec(ctx, `INSERT INTO profile_cosmetic_catalog
(id,kind,name,rarity,source,price,level,track,admin_only,colors) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (id) DO UPDATE SET kind=EXCLUDED.kind,name=EXCLUDED.name,rarity=EXCLUDED.rarity,
source=EXCLUDED.source,price=EXCLUDED.price,level=EXCLUDED.level,track=EXCLUDED.track,admin_only=EXCLUDED.admin_only,colors=EXCLUDED.colors`,
			c.ID, c.Kind, c.Name, c.Rarity, c.Source, c.Price, level, track, c.AdminOnly, colors)
		if err != nil {
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
