package open_city_sql

// ============================================================
// КОНФИГ OPEN CITY
// Все игровые значения правятся только в этом файле: каталог
// предметов, локаций, NPC, квестов, разовых наград, XP,
// характеристик и лимитов. Все строки игры — на английском;
// русские диалоги и описания квестов живут в dict.go и
// подключаются по ключам (TitleKey, DescKey и т.п.) — замена
// словаря = мгновенная смена языка.
// ============================================================

// ---------- Каталог предметов ----------
// Kind: material | food | weapon | ammo | quest
// Price — ориентировочная цена в DOC (для будущих магазинов).
// DOC с предметами не смешивается: баланс — в open_city_state,
// предметы — в open_city_inventory.
type ItemDef struct {
	ID    string
	Name  string // английское отображаемое имя
	Kind  string
	Price int
}

var Items = []ItemDef{
	{ID: "pistol", Name: "Pistol", Kind: "weapon", Price: 120},
	{ID: "rusty_pipe", Name: "Rusty Pipe", Kind: "weapon", Price: 15},
	{ID: "nail_knuckles", Name: "Nail Knuckles", Kind: "weapon", Price: 40},
	{ID: "pistol_ammo", Name: "Pistol Ammo", Kind: "ammo", Price: 5},
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
// Bounds — игровые границы координат; move их учитывает.
// Фронт живёт в пикселях мира 0..1600.
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

// ---------- NPC (заглушки-каталог для следующих этапов) ----------
type NPCDef struct {
	ID        string
	Name      string
	DialogKey string // ключ в dict.go
}

var NPCs = []NPCDef{
	{ID: "mayor", Name: "Mayor Cole", DialogKey: "npc.mayor.greet"},
	{ID: "mechanic", Name: "Mechanic Rosa", DialogKey: "npc.mechanic.greet"},
	{ID: "dealer", Name: "Dealer Slim", DialogKey: "npc.dealer.greet"},
}

// ---------- Квесты (каталог; статус хранится в БД) ----------
type QuestDef struct {
	ID        string
	TitleKey  string // русский текст в dict.go
	DescKey   string
	RewardDoc int // награда начисляется сервером по завершении
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
// Frontend отправляет только id награды; размер — фиксированный здесь.
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

// ---------- Прокачка ----------
const (
	StartLevel      = 1
	StartXP         = 0
	StartHealth     = 100
	StartMaxHealth  = 100
	XPBase          = 100  // XP на уровень N = XPBase * N
	MaxInventoryQty = 99   // максимум одного предмета в стопке
	MaxProgressJSON = 4096 // байт, лимит тела base_progress
)

// ---------- Прокачка: очки и характеристики ----------
// XP за действия фиксирован на сервере — клиент присылает только
// ключ источника. Левелапы, очки навыков и итоговые характеристики
// считает сервер (ComputeStats) — клиент лишь отображает.
const (
	SkillPointsPerLevel = 3  // очков за новый уровень
	LevelMaxHPGrowth    = 10 // +Max HP за каждый уровень
	MaxHPPerPoint       = 10 // +Max HP за одно очко в Max HP
	DamagePerPoint      = 2  // +к урону оружия за очко
	DefensePerPoint     = 2  // +% снижения входящего урона за очко
	DefenseCap          = 60 // потолок снижения урона, %
	SpeedPerPointPct    = 3  // +% скорости движения за очко
	AccuracyBase        = 85 // базовый шанс попадания, %
	AccuracyPerPoint    = 3  // +% за очко (потолок 100)
)

// XPRewards — XP за игровые действия (пока: убийства).
// Ключ источника знает и клиент (из id врага), и сервер.
var XPRewards = map[string]int{
	"kill.thug":  15,
	"kill.brute": 30,
}

// StatDefs — характеристики, куда игрок вкладывает очки (экран Skills).
type StatDef struct {
	ID   string
	Name string
}

var StatDefs = []StatDef{
	{ID: "max_hp", Name: "Max HP"},
	{ID: "damage", Name: "Damage"},
	{ID: "defense", Name: "Defense"},
	{ID: "speed", Name: "Speed"},
	{ID: "accuracy", Name: "Accuracy"},
}

var StatByID = map[string]bool{}

func init() {
	for _, s := range StatDefs {
		StatByID[s.ID] = true
	}
}

// ComputeStats — итоговые характеристики из уровня и вложенных очков.
// Max HP считается здесь в одном месте — и при левелапе, и при
// аллокации очка, поэтому значения не разъезжаются.
func ComputeStats(level int, pts map[string]int) map[string]interface{} {
	maxHP := StartMaxHealth + (level-1)*LevelMaxHPGrowth + pts["max_hp"]*MaxHPPerPoint
	defense := pts["defense"] * DefensePerPoint
	if defense > DefenseCap {
		defense = DefenseCap
	}
	accuracy := AccuracyBase + pts["accuracy"]*AccuracyPerPoint
	if accuracy > 100 {
		accuracy = 100
	}
	return map[string]interface{}{
		"max_hp":   maxHP,
		"damage":   pts["damage"] * DamagePerPoint,
		"defense":  defense,
		"speed":    100 + pts["speed"]*SpeedPerPointPct, // % от базовой скорости
		"accuracy": accuracy,
	}
}

// ---------- Разрешённые ключи base_progress ----------
// save_progress принимает только эти верхнеуровневые ключи —
// клиент не может записать в прогресс произвольные данные.
// Характеристики и экипировка сюда НЕ входят: у них свои колонки,
// их меняет только сервер.
var AllowedProgressKeys = map[string]bool{
	"flags":         true,
	"claimed":       true, // список выданных разовых наград
	"tutorial_step": true,
	"stats":         true,
}

// XPNeeded — сколько XP требуется для уровня level -> level+1.
func XPNeeded(level int) int {
	if level < 1 {
		level = 1
	}
	return XPBase * level
}
