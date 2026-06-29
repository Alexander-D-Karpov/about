package ranking

import "time"

type Tierlist struct {
	ID          int       `json:"id"`
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Published   bool      `json:"published"`
	CardStyle   string    `json:"card_style"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Tiers       []Tier    `json:"tiers"`
	Entries     []Entry   `json:"entries"`
}

type Tier struct {
	ID         int    `json:"id"`
	TierlistID int    `json:"tierlist_id"`
	Name       string `json:"name"`
	Color      string `json:"color"`
	Position   int    `json:"position"`
}

type Entry struct {
	ID         int    `json:"id"`
	TierlistID int    `json:"tierlist_id"`
	TierID     *int   `json:"tier_id"`
	Name       string `json:"name"`
	ImagePath  string `json:"image_path"`
	ThumbPath  string `json:"thumb_path"`
	Position   int    `json:"position"`
}

type SaveRequest struct {
	Title       string          `json:"title"`
	Description string          `json:"description"`
	CardStyle   string          `json:"card_style"`
	Tiers       []TierSaveData  `json:"tiers"`
	Entries     []EntrySaveData `json:"entries"`
}

type TierSaveData struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Color    string `json:"color"`
	Position int    `json:"position"`
}

type EntrySaveData struct {
	ID       int    `json:"id"`
	TierID   *int   `json:"tier_id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
}

var DefaultTiers = []struct {
	Name  string
	Color string
}{
	{"S", "#FF7F7F"},
	{"A", "#FFBF7F"},
	{"B", "#FFDF7F"},
	{"C", "#FFFF7F"},
	{"D", "#BFFF7F"},
	{"F", "#7FBFFF"},
}
