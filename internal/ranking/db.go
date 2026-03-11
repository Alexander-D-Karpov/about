package ranking

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

func openDB(pgURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", pgURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	return db, nil
}

func runMigrations(db *sql.DB) error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS tierlists (
			id SERIAL PRIMARY KEY,
			slug TEXT UNIQUE NOT NULL,
			title TEXT NOT NULL,
			description TEXT DEFAULT '',
			published BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS tiers (
			id SERIAL PRIMARY KEY,
			tierlist_id INTEGER NOT NULL REFERENCES tierlists(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			color TEXT NOT NULL DEFAULT '#CCCCCC',
			position INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS tier_entries (
			id SERIAL PRIMARY KEY,
			tierlist_id INTEGER NOT NULL REFERENCES tierlists(id) ON DELETE CASCADE,
			tier_id INTEGER REFERENCES tiers(id) ON DELETE SET NULL,
			name TEXT DEFAULT '',
			image_path TEXT DEFAULT '',
			thumb_path TEXT DEFAULT '',
			position INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tiers_tierlist ON tiers(tierlist_id)`,
		`CREATE INDEX IF NOT EXISTS idx_entries_tierlist ON tier_entries(tierlist_id)`,
		`CREATE INDEX IF NOT EXISTS idx_entries_tier ON tier_entries(tier_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tierlists_slug ON tierlists(slug)`,
		`CREATE INDEX IF NOT EXISTS idx_tierlists_published ON tierlists(published)`,
	}

	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %w\nSQL: %s", err, m)
		}
	}
	return nil
}
