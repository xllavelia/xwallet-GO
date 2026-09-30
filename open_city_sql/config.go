package open_city_sql

// ============================================================
// КОНФИГ OPEN CITY
// Каталоги: предметы, локации, NPC, квесты, награды, враги (XP),
// прокачка. Строки — английские ключи, русский текст в dict.go.
// ============================================================

// ---------- Каталог предметов ----------
// Kind: material | food | weapon | quest
type ItemDef struct {
	ID    string
	Name  string
	Kind  string
	Price int
}

var Items = []ItemDef{
	{ID: "rusty_pipe", Name: "Rusty Pipe", Kind: "weapon", Price: 15},
	{ID: "nail_knuckles", Name: "Nail Knuckles", Kind: "weapon", Price: 40},
	{ID: "apple_pie", Name: "Apple Pie", Kind: "food", Price: 5},
	{ID: "soda_can", Name: "Soda Can", Kind: "food", Price: 3},
	{ID: "scrap_metal", Name: "Scrap Metal", Kind: "material", Price: 2},
	{ID: "old_circuit", Name: "Old Circuit", Kind: "material", Price: 8},
	{ID: "mayor_letter", Name: "Mayor's Letter", Kind: "quest", Price: 0},
}

var ItemByID = map[string]*ItemDef{}

func init() {
	for i := range Items {
		ItemByID[Items[i].ID] = &Items[i]
	}
}

// ---------- Локации ----------
type LocationDef struct {
	ID   string
	Name string
	MinX float64
	MaxX float64
	MinY float64
	MaxY float64
}

var Locations = []LocationDef{
	{ID: "downtown", Name: "Downtown", MinX: 0, MaxX: 1600, MinY: 0, MaxY: 1600},
	{ID: "industrial", Name: "Industrial District", MinX: 0, MaxX: 1600, MinY: 0, MaxY: 1600},
	{ID: "suburbs", Name: "Suburbs", MinX: 0, MaxX: 1600, MinY: 0, MaxY: 1600},
}

var LocationByID = map[string]*LocationDef{}

func init() {
	for i := range Locations {
		LocationByID[Locations[i].ID] = &Locations[i]
	}
}

// ---------- NPC ----------
type NPCDef struct {
	ID        string
	Name      string
	DialogKey string
}

var NPCs = []NPCDef{
	{ID: "mayor", Name: "Mayor Cole", DialogKey: "npc.mayor.greet"},
	{ID: "mechanic", Name: "Mechanic Rosa", DialogKey: "npc.mechanic.greet"},
	{ID: "dealer", Name: "Dealer Slim", DialogKey: "npc.dealer.greet"},
}

// ---------- Квесты ----------
type QuestDef struct {
	ID        string
	TitleKey  string
	DescKey   string
	RewardDoc int
}

var Quests = []QuestDef{
	{ID: "welcome", TitleKey: "quest.welcome.title", DescKey: "quest.welcome.desc", RewardDoc: 25},
}

var QuestByID = map[string]*QuestDef{}

func init() {
	for i := range Quests {
		QuestByID[Quests[i].ID] = &Quests[i]
	}
}

// ---------- Разовые награды ----------
type RewardDef struct {
	ID  string
	Doc int
}

var Rewards = []RewardDef{
	{ID: "first_visit", Doc: 50},
	{ID: "tutorial_done", Doc: 10},
}

var RewardByID = map[string]*RewardDef{}

func init() {
	for i := range Rewards {
		RewardByID[Rewards[i].ID] = &Rewards[i]
	}
}

// ---------- Враги: XP за убийство ----------
// Характеристики врагов живут на клиенте; сервер знает только тип и XP.
type EnemyDef struct {
	ID   string
	Name string
	XP   int
}

var Enemies = []EnemyDef{
	{ID: "thug", Name: "Thug", XP: 20},
	{ID: "brute", Name: "Brute", XP: 45},
}

var EnemyByID = map[string]*EnemyDef{}

func init() {
	for i := range Enemies {
		EnemyByID[Enemies[i].ID] = &Enemies[i]
	}
}

// ---------- Прокачка ----------
const (
	StartLevel      = 1
	StartXP         = 0
	StartHealth     = 100
	StartMaxHealth  = 100
	XPBase          = 100 // XP на уровень N = XPBase * N
	MaxInventoryQty = 99
	MaxProgressJSON = 4096

	SkillPointsPerLevel = 3 // очков за новый уровень
	StatMax             = 50
	MaxHPBase           = 100
	MaxHPPerPoint       = 10
	DamagePerPoint      = 2
	DefensePerPoint     = 1
	SpeedPerPoint       = 8
	AccuracyPerPointPct = 4 // -4% к cooldown за очко (мин. -40%)
)

// AllocatableStats — характеристики, в которые можно вкладывать очки.
var AllocatableStats = map[string]bool{
	"max_hp": true, "damage": true, "defense": true,
	"speed": true, "accuracy": true,
}

// DefaultStats — стартовые характеристики персонажа.
func DefaultStats() map[string]interface{} {
	return map[string]interface{}{
		"max_hp":       MaxHPBase,
		"damage":       0,
		"defense":      0,
		"speed":        0,
		"accuracy":     0,
		"skill_points": 0,
	}
}

// ---------- Разрешённые ключи base_progress ----------
var AllowedProgressKeys = map[string]bool{
	"flags":         true,
	"claimed":       true,
	"tutorial_step": true,
	"stats":         true,
	"equipment":     true, // {"weapon": "pistol"} — экипировка
}

// XPNeeded — сколько XP требуется для уровня level -> level+1.
func XPNeeded(level int) int {
	if level < 1 {
		level = 1
	}
	return XPBase * level
}
