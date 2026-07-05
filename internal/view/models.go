package view

import "html/template"

type PageVM struct {
	Theme    string
	Top      TopVM
	Profile  ProfileVM
	Health   []HealthCardVM
	Music    MusicVM
	Code     CodeVM
	Tech     []TechVM
	Games    GamesVM
	Travel   TravelVM
	Hosting  HostingVM
	Machines MachinesVM
	Meme     MemeVM
	Projects []ProjectVM
}

type TopVM struct {
	Webring WebringVM
	Status  StatusVM
}

type StatusVM struct {
	Online        bool
	ClockInit     string
	UptimeSeconds int64
	VisitsTotal   string
	VisitorsToday int
}

type WebringPeer struct {
	Name    string
	Emoji   string
	URL     string
	Favicon string
}

type WebringVM struct {
	BaseURL string
	Prev    WebringPeer
	Next    WebringPeer
}

type Seg struct {
	Text  string
	Color string
}

type SocialVM struct {
	Label string
	Icon  string
	Href  string
	Color string
}

type ProfileVM struct {
	Avatar  string
	Name    string
	Role    []Seg
	Stack   []Seg
	Bio     string
	Socials []SocialVM
	TierURL string
}

type HealthCardVM struct {
	Label string
	Value string
	Unit  string
	Sub   string
	Color string
	Icon  string
	Kind  string
	Data  []float64
}

type TotalVM struct {
	Label string
	Value string
	Color template.CSS
	Delta string
	Spark []int
}

type NowVM struct {
	Playing      bool
	Title        string
	Artist       string
	Album        string
	Art          string
	ProgressPct  string
	Elapsed      string
	Duration     string
	ElapsedSec   int
	DurationSec  int
	LastfmURL    string
	StartedAt    int64
	LovedLastfm  bool
	LikedSpotify bool
}

type RecentTrackVM struct {
	Title        string
	Artist       string
	Ago          string
	Initial      string
	Bg           string
	Color        string
	Image        string
	Loved        bool
	LikedSpotify bool
}

type MusicVM struct {
	Totals            []TotalVM
	Now               NowVM
	Recent            []RecentTrackVM
	TopArtists        []CoverVM
	TopAlbums         []CoverVM
	WeeklyJSON        template.JS
	WeeklyPeak        string
	Tags              []TagVM
	SpotifyConnected  bool
	SpotifyLikedCount string
	LovedCount        string
}

type CoverVM struct {
	Name  string
	Plays string
	Image string
	Bg    template.CSS
}

type TagVM struct {
	Name   string
	Plays  string
	SizePx int
	Color  string
}

type CodeTotalVM struct {
	Value  string
	Label  string
	Accent string
	Chip   string
	Icon   string
}

type CellVM struct {
	Color string
	Title string
}

type StatVM struct {
	Value string
	Unit  string
	Label string
	Color string
}

type LangVM struct {
	Name   string
	Pct    string
	Color  string
	BarPct string
}

type ActivityVM struct {
	Icon    string
	Color   string
	Text    string
	Meta    string
	HasDiff bool
	Add     string
	Del     string
	DiffG   int
}

type WeekVM struct {
	Name  string
	Time  string
	Width string
	Color string
}

type CodeVM struct {
	Totals       []CodeTotalVM
	ContribTitle string
	ContribCells []CellVM
	ContribStats []StatVM
	HeatEmpty    string
	Langs        []LangVM
	Activity     []ActivityVM
	WeekTotal    string
	ThisWeek     []WeekVM
}

type TechVM struct {
	Name string
	Icon string
}

type CurrentGameVM struct {
	Cover    string
	Name     string
	Session  string
	Total    string
	Chapter  string
	AchDone  int
	AchTotal int
	AchPct   string
	SteamURL string
}

type GameSmallVM struct {
	Name    string
	Weeks   string
	Total   string
	Image   string
	Initial string
	Bg      string
	Color   string
}

type TopGameVM struct {
	Rank    string
	Name    string
	Hours   string
	Image   string
	Initial string
	Bg      string
	Color   string
}

type GenreVM struct {
	Name  string
	Hours string
	Share string
	Color string
}

type PlatformVM struct {
	Name  string
	Color string
	Value float64
}

type BLMapVM struct {
	Initial   string
	Name      string
	Diff      string
	DiffColor string
	Acc       string
	PP        string
	Image     string
	Replay    string
}

type BeatLeaderVM struct {
	PP          string
	RankGlobal  string
	RankRU      string
	AvgAcc      string
	RankedPlays string
	Cubes       string
	Maps        []BLMapVM
}

type GamesVM struct {
	InGame       bool
	Current      CurrentGameVM
	RecentGames  []GameSmallVM
	TopGames     []TopGameVM
	Genres       []GenreVM
	Platforms    []PlatformVM
	PlatformJSON template.JS
	BL           BeatLeaderVM
}

type RideVM struct {
	Name  string
	Date  string
	Km    string
	Elev  string
	Time  string
	Speed string
	Color string
	Prof  []float64
}

type BikeTotalsVM struct {
	Distance  string
	Rides     int
	Elevation string
	Time      string
	Avg       string
	Longest   string
}

type AlbumVM struct {
	Name  string
	Count string
	Cover string
}

type TravelVM struct {
	Totals       BikeTotalsVM
	Rides        []RideVM
	BikeJSON     template.JS
	PlacesJSON   template.JS
	PlacesConfig template.JS
	Albums       []AlbumVM
	PlacesCount  string
}

type ServiceVM struct {
	Name   string
	Desc   string
	Ping   string
	Icon   string
	Status string
	URL    string
}

type HostingVM struct {
	Services []ServiceVM
	Online   int
	Total    int
}

type KV struct {
	K string
	V string
}

type MachinesVM struct {
	Machines []MachineVM
	Active   string
	Meme     MemeVM
}

type MemeVM struct {
	Type   string
	Image  string
	Text   string
	Source string
}

type ProjectVM struct {
	Name   string
	Desc   string
	Image  string
	Source string
	Demo   string
	Tags   []string
}

type MachineVM struct {
	Key      string
	Label    string
	User     string
	Host     string
	Rule     string
	ASCII    string
	Rows     []KV
	Machines []MachineVM
	Active   string
	Meme     MemeVM
}

type AdminPluginVM struct {
	Name        string
	Enabled     bool
	Order       int
	Section     string
	HasSettings bool
}

type AdminVM struct {
	Plugins []AdminPluginVM
	Saved   bool
}
