package ranking

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Store struct {
	db        *sql.DB
	mediaPath string
	cache     map[string]*Tierlist
	mu        sync.RWMutex
}

func NewStore(pgURL, mediaPath string) (*Store, error) {
	db, err := openDB(pgURL)
	if err != nil {
		return nil, err
	}
	if err := runMigrations(db); err != nil {
		return nil, err
	}
	s := &Store{
		db:        db,
		mediaPath: mediaPath,
		cache:     make(map[string]*Tierlist),
	}
	if err := s.loadCache(); err != nil {
		return nil, fmt.Errorf("failed to load cache: %w", err)
	}
	return s, nil
}

func (s *Store) Close() {
	s.db.Close()
}

func (s *Store) loadCache() error {
	rows, err := s.db.Query(`SELECT id, slug, title, description, published, card_style, created_at, updated_at FROM tierlists ORDER BY created_at DESC`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		tl := &Tierlist{}
		if err := rows.Scan(&tl.ID, &tl.Slug, &tl.Title, &tl.Description, &tl.Published, &tl.CardStyle, &tl.CreatedAt, &tl.UpdatedAt); err != nil {
			return err
		}
		if err := s.loadTierlistRelations(tl); err != nil {
			log.Printf("[Ranking] Failed to load relations for %s: %v", tl.Slug, err)
			continue
		}
		s.cache[tl.Slug] = tl
	}
	log.Printf("[Ranking] Loaded %d tierlists into cache", len(s.cache))
	return nil
}

func (s *Store) loadTierlistRelations(tl *Tierlist) error {
	tierRows, err := s.db.Query(`SELECT id, tierlist_id, name, color, position FROM tiers WHERE tierlist_id = $1 ORDER BY position`, tl.ID)
	if err != nil {
		return err
	}
	defer tierRows.Close()

	tl.Tiers = nil
	for tierRows.Next() {
		t := Tier{}
		if err := tierRows.Scan(&t.ID, &t.TierlistID, &t.Name, &t.Color, &t.Position); err != nil {
			return err
		}
		tl.Tiers = append(tl.Tiers, t)
	}

	entryRows, err := s.db.Query(`SELECT id, tierlist_id, tier_id, name, image_path, thumb_path, position FROM tier_entries WHERE tierlist_id = $1 ORDER BY position`, tl.ID)
	if err != nil {
		return err
	}
	defer entryRows.Close()

	tl.Entries = nil
	for entryRows.Next() {
		e := Entry{}
		var tierID sql.NullInt64
		if err := entryRows.Scan(&e.ID, &e.TierlistID, &tierID, &e.Name, &e.ImagePath, &e.ThumbPath, &e.Position); err != nil {
			return err
		}
		if tierID.Valid {
			id := int(tierID.Int64)
			e.TierID = &id
		}
		tl.Entries = append(tl.Entries, e)
	}
	return nil
}

func (s *Store) GetAllPublished() []*Tierlist {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Tierlist
	for _, tl := range s.cache {
		if tl.Published {
			result = append(result, tl)
		}
	}
	return result
}

func (s *Store) GetAll() []*Tierlist {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Tierlist
	for _, tl := range s.cache {
		result = append(result, tl)
	}
	return result
}

func (s *Store) GetBySlug(slug string) *Tierlist {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache[slug]
}

var slugRegex = regexp.MustCompile(`[^a-z0-9-]+`)

func generateSlug(title string) string {
	slug := strings.ToLower(strings.TrimSpace(title))
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = slugRegex.ReplaceAllString(slug, "")
	if len(slug) > 60 {
		slug = slug[:60]
	}
	if slug == "" {
		slug = "tierlist"
	}
	return slug
}

func (s *Store) Create(title, description string) (*Tierlist, error) {
	slug := generateSlug(title)

	s.mu.RLock()
	baseSlug := slug
	counter := 1
	for s.cache[slug] != nil {
		slug = fmt.Sprintf("%s-%d", baseSlug, counter)
		counter++
	}
	s.mu.RUnlock()

	now := time.Now()
	tl := &Tierlist{
		Slug:        slug,
		Title:       title,
		Description: description,
		Published:   false,
		CardStyle:   "square",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	err := s.db.QueryRow(
		`INSERT INTO tierlists (slug, title, description, published, card_style, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		tl.Slug, tl.Title, tl.Description, tl.Published, tl.CardStyle, tl.CreatedAt, tl.UpdatedAt,
	).Scan(&tl.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to insert tierlist: %w", err)
	}

	for i, dt := range DefaultTiers {
		tier := Tier{TierlistID: tl.ID, Name: dt.Name, Color: dt.Color, Position: i}
		err := s.db.QueryRow(
			`INSERT INTO tiers (tierlist_id, name, color, position) VALUES ($1, $2, $3, $4) RETURNING id`,
			tier.TierlistID, tier.Name, tier.Color, tier.Position,
		).Scan(&tier.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to insert default tier: %w", err)
		}
		tl.Tiers = append(tl.Tiers, tier)
	}

	s.mu.Lock()
	s.cache[tl.Slug] = tl
	s.mu.Unlock()

	return tl, nil
}

func (s *Store) Save(slug string, req SaveRequest) error {
	s.mu.RLock()
	tl := s.cache[slug]
	s.mu.RUnlock()
	if tl == nil {
		return fmt.Errorf("tierlist not found: %s", slug)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	style := req.CardStyle
	if style != "vertical" && style != "horizontal" && style != "square" {
		style = "square"
	}

	now := time.Now()
	_, err = tx.Exec(`UPDATE tierlists SET title=$1, description=$2, card_style=$3, updated_at=$4 WHERE id=$5`,
		req.Title, req.Description, style, now, tl.ID)
	if err != nil {
		return err
	}

	reqTierIDs := make(map[int]bool)
	for _, t := range req.Tiers {
		if t.ID > 0 {
			reqTierIDs[t.ID] = true
		}
	}

	s.mu.RLock()
	currentTiers := make([]Tier, len(tl.Tiers))
	copy(currentTiers, tl.Tiers)
	s.mu.RUnlock()

	for _, t := range currentTiers {
		if !reqTierIDs[t.ID] {
			_, _ = tx.Exec(`UPDATE tier_entries SET tier_id = NULL WHERE tier_id = $1`, t.ID)
			_, _ = tx.Exec(`DELETE FROM tiers WHERE id = $1`, t.ID)
		}
	}

	tierStmt, err := tx.Prepare(`UPDATE tiers SET name=$1, color=$2, position=$3 WHERE id=$4`)
	if err != nil {
		return err
	}
	defer tierStmt.Close()

	for _, t := range req.Tiers {
		if t.ID > 0 {
			_, err = tierStmt.Exec(t.Name, t.Color, t.Position, t.ID)
			if err != nil {
				return err
			}
		} else {
			_, err = tx.Exec(`INSERT INTO tiers (tierlist_id, name, color, position) VALUES ($1, $2, $3, $4)`,
				tl.ID, t.Name, t.Color, t.Position)
			if err != nil {
				return err
			}
		}
	}

	entryStmt, err := tx.Prepare(`UPDATE tier_entries SET tier_id=$1, name=$2, position=$3 WHERE id=$4`)
	if err != nil {
		return err
	}
	defer entryStmt.Close()

	for _, e := range req.Entries {
		if e.ID > 0 {
			_, err = entryStmt.Exec(e.TierID, e.Name, e.Position, e.ID)
			if err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	reloaded := &Tierlist{ID: tl.ID}
	if err := s.loadTierlistRelations(reloaded); err != nil {
		log.Printf("[Ranking] Failed to reload relations after save for %s: %v", slug, err)
	}

	s.mu.Lock()
	tl.Title = req.Title
	tl.Description = req.Description
	tl.CardStyle = style
	tl.UpdatedAt = now
	if reloaded.Tiers != nil {
		tl.Tiers = reloaded.Tiers
	}
	if reloaded.Entries != nil {
		tl.Entries = reloaded.Entries
	}
	s.mu.Unlock()

	return nil
}

func (s *Store) SetPublished(slug string, published bool) error {
	s.mu.RLock()
	tl := s.cache[slug]
	s.mu.RUnlock()
	if tl == nil {
		return fmt.Errorf("tierlist not found")
	}

	_, err := s.db.Exec(`UPDATE tierlists SET published=$1, updated_at=$2 WHERE id=$3`, published, time.Now(), tl.ID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	tl.Published = published
	tl.UpdatedAt = time.Now()
	s.mu.Unlock()

	return nil
}

func (s *Store) Delete(slug string) error {
	s.mu.RLock()
	tl := s.cache[slug]
	s.mu.RUnlock()
	if tl == nil {
		return fmt.Errorf("tierlist not found")
	}

	_, err := s.db.Exec(`DELETE FROM tierlists WHERE id=$1`, tl.ID)
	if err != nil {
		return err
	}

	dir := filepath.Join(s.mediaPath, "ranking", slug)
	if err := os.RemoveAll(dir); err != nil {
		log.Printf("[Ranking] Failed to remove media for %s: %v", slug, err)
	}

	s.mu.Lock()
	delete(s.cache, slug)
	s.mu.Unlock()

	return nil
}

func (s *Store) AddEntry(tierlistID int, slug string, entry Entry) (*Entry, error) {
	var tierID *int
	if entry.TierID != nil && *entry.TierID > 0 {
		tierID = entry.TierID
	}

	s.mu.Lock()
	tl := s.cache[slug]
	if tl != nil {
		maxPos := 0
		for _, e := range tl.Entries {
			if e.Position >= maxPos {
				maxPos = e.Position + 1
			}
		}
		entry.Position = maxPos
	}
	s.mu.Unlock()

	err := s.db.QueryRow(
		`INSERT INTO tier_entries (tierlist_id, tier_id, name, image_path, thumb_path, position) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		tierlistID, tierID, entry.Name, entry.ImagePath, entry.ThumbPath, entry.Position,
	).Scan(&entry.ID)
	if err != nil {
		return nil, err
	}
	entry.TierlistID = tierlistID

	s.mu.Lock()
	if tl := s.cache[slug]; tl != nil {
		found := false
		for _, e := range tl.Entries {
			if e.ID == entry.ID {
				found = true
				break
			}
		}
		if !found {
			tl.Entries = append(tl.Entries, entry)
		}
	}
	s.mu.Unlock()

	return &entry, nil
}

func (s *Store) DeleteEntry(slug string, entryID int) error {
	s.mu.RLock()
	tl := s.cache[slug]
	s.mu.RUnlock()
	if tl == nil {
		return fmt.Errorf("tierlist not found")
	}

	var entry *Entry
	s.mu.RLock()
	for i := range tl.Entries {
		if tl.Entries[i].ID == entryID {
			entry = &tl.Entries[i]
			break
		}
	}
	s.mu.RUnlock()

	_, err := s.db.Exec(`DELETE FROM tier_entries WHERE id = $1 AND tierlist_id = $2`, entryID, tl.ID)
	if err != nil {
		return err
	}

	if entry != nil {
		removeMediaFile(s.mediaPath, entry.ImagePath)
		removeMediaFile(s.mediaPath, entry.ThumbPath)
	}

	s.mu.Lock()
	newEntries := make([]Entry, 0, len(tl.Entries))
	for _, e := range tl.Entries {
		if e.ID != entryID {
			newEntries = append(newEntries, e)
		}
	}
	tl.Entries = newEntries
	s.mu.Unlock()

	return nil
}

func removeMediaFile(mediaPath, webPath string) {
	if webPath == "" {
		return
	}
	rel := strings.TrimPrefix(webPath, "/media/")
	if rel == webPath {
		return
	}
	full := filepath.Join(mediaPath, rel)
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		log.Printf("[Ranking] Failed to remove %s: %v", full, err)
	}
}

func (s *Store) AddTier(tierlistID int, slug string, tier Tier) (*Tier, error) {
	s.mu.RLock()
	if tl := s.cache[slug]; tl != nil {
		maxPos := -1
		for _, t := range tl.Tiers {
			if t.Position > maxPos {
				maxPos = t.Position
			}
		}
		tier.Position = maxPos + 1
	}
	s.mu.RUnlock()

	err := s.db.QueryRow(
		`INSERT INTO tiers (tierlist_id, name, color, position) VALUES ($1, $2, $3, $4) RETURNING id`,
		tierlistID, tier.Name, tier.Color, tier.Position,
	).Scan(&tier.ID)
	if err != nil {
		return nil, err
	}
	tier.TierlistID = tierlistID

	s.mu.Lock()
	if tl := s.cache[slug]; tl != nil {
		found := false
		for _, t := range tl.Tiers {
			if t.ID == tier.ID {
				found = true
				break
			}
		}
		if !found {
			tl.Tiers = append(tl.Tiers, tier)
		}
	}
	s.mu.Unlock()

	return &tier, nil
}

func (s *Store) RegenThumb(entryID int, thumbPath string) error {
	_, err := s.db.Exec(`UPDATE tier_entries SET thumb_path=$1 WHERE id=$2`, thumbPath, entryID)
	return err
}

func (s *Store) reloadOne(slug string) error {
	s.mu.RLock()
	tl := s.cache[slug]
	s.mu.RUnlock()
	if tl == nil {
		return fmt.Errorf("not found")
	}
	reloaded := &Tierlist{ID: tl.ID}
	if err := s.loadTierlistRelations(reloaded); err != nil {
		return err
	}
	s.mu.Lock()
	tl.Tiers = reloaded.Tiers
	tl.Entries = reloaded.Entries
	s.mu.Unlock()
	return nil
}
