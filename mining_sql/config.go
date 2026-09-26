package mining_sql

import "time"

// ===================== ОБЩИЕ КОНСТАНТЫ =====================

const (
	// Сколько копий ОДНОГО и того же сервера (по catalog_id) может держать один игрок без расширителя слотов.
	BaseMaxCopiesPerServer = 15

	// Сколько дополнительных копий даёт один "расширитель слотов" из магазина. Складывается при повторной покупке.
	SlotExpanderAddSlots = 5
	// Цена расширителя слотов.
	SlotExpanderPrice = 200.0

	// Энергия у нового игрока при первом входе в майнинг.
	DefaultStartEnergy = 200.0
	// Максимум энергии у нового игрока при первом входе.
	DefaultMaxEnergy = 200.0

	// Сколько добавляет "расширитель максимальной энергии" за одну покупку. Складывается.
	MaxEnergyExpanderAddAmount = 50.0
	// Цена расширителя максимальной энергии.
	MaxEnergyExpanderPrice = 150.0

	// Если игрок не заходил очень долго — тик считает прогресс не больше этого времени,
	// чтобы не улетать в отрицательную энергию/безумные цифры при возвращении через неделю.
	MaxCatchUpDuration = 12 * time.Hour
)

// ===================== РЕДКОСТИ =====================

type ServerRarity string

const (
	RarityEpic      ServerRarity = "epic"      // 6 серверов
	RarityMythic    ServerRarity = "mythic"    // 6 серверов
	RarityLegendary ServerRarity = "legendary" // 3 сервера, никогда не спят
)

// ===================== ПЕРКИ =====================

// Перк — небольшой пассивный бонус/минус сервера. Мультипликаторы применяются
// поверх ранга и баффов (перемножаются). 1.0 = без эффекта.
type Perk struct {
	Key         string  // ключ, используется в ServerDef.Perks
	Name        string  // отображаемое имя
	Description string  // описание для карточки сервера
	ProfitMult  float64 // множитель дохода/час
	EnergyMult  float64 // множитель расхода энергии/час
}

// Каталог всех перков. Можно менять описания/цифры или добавлять новые — просто
// ссылайся на Key в поле Perks у сервера.
var PerkDefs = map[string]Perk{
	"efficient": { // slight energy savings
		Key: "efficient", Name: "Energy Efficiency",
		Description: "Consumes 8% less energy.",
		ProfitMult:  1.0, EnergyMult: 0.92,
	},

	"overclocked": { // slight profit boost
		Key: "overclocked", Name: "Overclocking",
		Description: "Generates 8% more income.",
		ProfitMult:  1.08, EnergyMult: 1.0,
	},

	"coldroom": { // cooled server room — greater energy savings
		Key: "coldroom", Name: "Cold Server Room",
		Description: "Consumes 12% less energy.",
		ProfitMult:  1.0, EnergyMult: 0.88,
	},

	"turbo": { // good profit boost, but slightly higher consumption
		Key: "turbo", Name: "Turbo Mode",
		Description: "Generates 12% more income but uses 5% more energy.",
		ProfitMult:  1.12, EnergyMult: 1.06,
	},

	"stable": { // balanced perk
		Key: "stable", Name: "Stable Operation",
		Description: "4% more income and 4% less energy consumption.",
		ProfitMult:  1.04, EnergyMult: 0.96,
	},

	"silent": { // energy savings only
		Key: "silent", Name: "Silent Mode",
		Description: "Consumes 6% less energy.",
		ProfitMult:  1.01, EnergyMult: 0.94,
	},
}

// ===================== РАНГИ (1..10) =====================

// RankTiers[i] описывает эффект ранга i+1 (индекс 0 = ранг 1 — стартовый, без доплаты).
// UpgradeCostMult — множитель к ServerDef.Price, чтобы получить цену прокачки НА этот ранг.
type RankTier struct {
	Level           int
	UpgradeCostMult float64 // цена прокачки = ServerDef.Price * UpgradeCostMult
	PowerMult       float64 // множитель отображаемой мощности
	ProfitMult      float64 // множитель дохода/час
	EnergyMult      float64 // множитель расхода энергии/час
}

var RankTiers = [10]RankTier{
	{Level: 1, UpgradeCostMult: 0.0, PowerMult: 1.00, ProfitMult: 1.00, EnergyMult: 1.00}, // старт, бесплатно
	{Level: 2, UpgradeCostMult: 0.4, PowerMult: 1.08, ProfitMult: 1.12, EnergyMult: 1.03},
	{Level: 3, UpgradeCostMult: 0.8, PowerMult: 1.16, ProfitMult: 1.25, EnergyMult: 1.06},
	{Level: 4, UpgradeCostMult: 1.4, PowerMult: 1.25, ProfitMult: 1.40, EnergyMult: 1.10},
	{Level: 5, UpgradeCostMult: 2.2, PowerMult: 1.35, ProfitMult: 1.55, EnergyMult: 1.14},
	{Level: 6, UpgradeCostMult: 3.2, PowerMult: 1.45, ProfitMult: 1.72, EnergyMult: 1.18},
	{Level: 7, UpgradeCostMult: 4.5, PowerMult: 1.55, ProfitMult: 1.90, EnergyMult: 1.22},
	{Level: 8, UpgradeCostMult: 6.0, PowerMult: 1.70, ProfitMult: 2.10, EnergyMult: 1.26},
	{Level: 9, UpgradeCostMult: 8.0, PowerMult: 1.85, ProfitMult: 2.35, EnergyMult: 1.30},
	{Level: 10, UpgradeCostMult: 10.5, PowerMult: 2.00, ProfitMult: 2.60, EnergyMult: 1.35}, // макс. ранг
}

// ===================== КАТАЛОГ СЕРВЕРОВ =====================

type ServerDef struct {
	ID      string       // стабильный ключ, НЕ менять после релиза (хранится в БД как catalog_id)
	Name    string       // отображаемое имя
	Country string       // страна для флажка/подписи
	Rarity  ServerRarity // epic / mythic / legendary

	Price float64 // цена покупки одной копии, $

	Power         float64 // отображаемая "мощность" на ранге 1, без баффов
	ProfitPerHour float64 // доход $/час на ранге 1, без баффов
	EnergyPerHour float64 // расход энергии/час на ранге 1, без баффов

	SleepMinutes int // сколько минут работает после "разбудить"; 0 = никогда не спит (Always On)

	LifetimeProfitCap float64 // максимум $ которые ОДНА копия может заработать за всю жизнь (потом exhausted)

	PermanentNoSleepAtMaxRank bool // true => на ранге 10 сервер навсегда перестаёт спать (Always On)

	Perks []string // ключи из PerkDefs, 0..N штук
}

// 15 серверов: 6 epic, 6 mythic, 3 legendary. Цены/мощности/энергия — заглушки
// (одинаковые внутри редкости специально, как ты просил), меняй смело по одному.
var ServerCatalog = []ServerDef{
	// ---------- EPIC (6) ----------
	{
		ID: "epic_ru", Name: "Сибирский Узел", Country: "Russia", Rarity: RarityEpic,
		Price: 30, Power: 10, ProfitPerHour: 2, EnergyPerHour: 3,
		SleepMinutes: 20, LifetimeProfitCap: 90,
		PermanentNoSleepAtMaxRank: false,
		Perks:                     []string{"stable"},
	},
	{
		ID: "epic_fr", Name: "Le Serveur Rouge", Country: "France", Rarity: RarityEpic,
		Price: 70, Power: 20, ProfitPerHour: 6, EnergyPerHour: 5,
		SleepMinutes: 25, LifetimeProfitCap: 110,
		PermanentNoSleepAtMaxRank: false,
		Perks:                     []string{"overclocked"},
	},
	{
		ID: "epic_it", Name: "Nodo Vesuvio", Country: "Italy", Rarity: RarityEpic,
		Price: 75, Power: 20, ProfitPerHour: 4, EnergyPerHour: 2,
		SleepMinutes: 30, LifetimeProfitCap: 110,
		PermanentNoSleepAtMaxRank: false,
		Perks:                     []string{"coldroom"},
	},
	{
		ID: "epic_de", Name: "Falkenrechner", Country: "Germany", Rarity: RarityEpic,
		Price: 80, Power: 30, ProfitPerHour: 7, EnergyPerHour: 6,
		SleepMinutes: 20, LifetimeProfitCap: 120,
		PermanentNoSleepAtMaxRank: false,
		Perks:                     []string{"efficient"},
	},
	{
		ID: "epic_jp", Name: "Ganges Monolith", Country: "India", Rarity: RarityEpic,
		Price: 85, Power: 25, ProfitPerHour: 6, EnergyPerHour: 3,
		SleepMinutes: 25, LifetimeProfitCap: 125,
		PermanentNoSleepAtMaxRank: true,
		Perks:                     []string{"silent"},
	},
	{
		ID: "epic_us", Name: "Liberty Rig", Country: "USA", Rarity: RarityEpic,
		Price: 100, Power: 40, ProfitPerHour: 10, EnergyPerHour: 9,
		SleepMinutes: 10, LifetimeProfitCap: 140,
		PermanentNoSleepAtMaxRank: false,
		Perks:                     []string{"turbo"},
	},
	// ---------- MYTHIC (6) ----------
	{
		ID: "mythic_gb", Name: "Crown Cluster", Country: "UK", Rarity: RarityMythic,
		Price: 150, Power: 70, ProfitPerHour: 15, EnergyPerHour: 11,
		SleepMinutes: 25, LifetimeProfitCap: 180,
		PermanentNoSleepAtMaxRank: false,
		Perks:                     []string{"efficient", "stable"},
	},
	{
		ID: "mythic_nl", Name: "Tulip Datacenter", Country: "Netherlands", Rarity: RarityMythic,
		Price: 160, Power: 80, ProfitPerHour: 17, EnergyPerHour: 13,
		SleepMinutes: 25, LifetimeProfitCap: 200,
		PermanentNoSleepAtMaxRank: false,
		Perks:                     []string{"silent", "turbo"},
	},
	{
		ID: "mythic_br", Name: "Amazônia Rig", Country: "Brazil", Rarity: RarityMythic,
		Price: 180, Power: 90, ProfitPerHour: 20, EnergyPerHour: 18,
		SleepMinutes: 15, LifetimeProfitCap: 190,
		PermanentNoSleepAtMaxRank: false,
		Perks:                     []string{"stable", "efficient"},
	},
	{
		ID: "mythic_ca", Name: "Maple Farm", Country: "Canada", Rarity: RarityMythic,
		Price: 190, Power: 40, ProfitPerHour: 9, EnergyPerHour: 3,
		SleepMinutes: 40, LifetimeProfitCap: 250,
		PermanentNoSleepAtMaxRank: true,
		Perks:                     []string{"coldroom", "silent"},
	},
	{
		ID: "mythic_kr", Name: "Hangang Node", Country: "South Korea", Rarity: RarityMythic,
		Price: 210, Power: 100, ProfitPerHour: 24, EnergyPerHour: 20,
		SleepMinutes: 20, LifetimeProfitCap: 190,
		PermanentNoSleepAtMaxRank: true,
		Perks:                     []string{"overclocked", "turbo"},
	},
	{
		ID: "mythic_cn", Name: "Great Wall Cluster", Country: "China", Rarity: RarityMythic,
		Price: 220, Power: 110, ProfitPerHour: 28, EnergyPerHour: 22,
		SleepMinutes: 20, LifetimeProfitCap: 200,
		PermanentNoSleepAtMaxRank: true,
		Perks:                     []string{"overclocked", "stable"},
	},

	// ---------- LEGENDARY (3) — никогда не спят (SleepMinutes: 0) ----------
	{
		ID: "legend_in", Name: "Sakura Node", Country: "Japan", Rarity: RarityLegendary,
		Price: 500, Power: 200, ProfitPerHour: 50, EnergyPerHour: 37,
		SleepMinutes: 0, LifetimeProfitCap: 300,
		PermanentNoSleepAtMaxRank: false, // уже Always On с самого начала
		Perks:                     []string{"efficient", "overclocked"},
	},
	{
		ID: "legend_is", Name: "Aurora Vault", Country: "Iceland", Rarity: RarityLegendary,
		Price: 900, Power: 350, ProfitPerHour: 70, EnergyPerHour: 50,
		SleepMinutes: 0, LifetimeProfitCap: 350,
		PermanentNoSleepAtMaxRank: false,
		Perks:                     []string{"stable", "silent"},
	},
	{
		ID: "legend_ch", Name: "Matterhorn Core", Country: "Switzerland", Rarity: RarityLegendary,
		Price: 1500, Power: 500, ProfitPerHour: 100, EnergyPerHour: 80,
		SleepMinutes: 0, LifetimeProfitCap: 500,
		PermanentNoSleepAtMaxRank: false,
		Perks:                     []string{"overclocked", "turbo"},
	},
}

// Быстрый доступ по ID, строится один раз в init().
var catalogIndex map[string]ServerDef

func init() {
	catalogIndex = make(map[string]ServerDef, len(ServerCatalog))
	for _, def := range ServerCatalog {
		catalogIndex[def.ID] = def
	}
}

func GetServerDef(catalogID string) (ServerDef, bool) {
	def, ok := catalogIndex[catalogID]
	return def, ok
}

// ===================== ПРЕДМЕТЫ МАГАЗИНА =====================

// Ключи баффов — хранятся в таблице mining_buffs.buff_type.
const (
	BuffNoSleep      = "no_sleep"        // сервер не засыпает всё время действия баффа
	BuffProfitBoost  = "profit_boost"    // x2 доход, энергия не меняется
	BuffPowerBoost   = "power_boost"     // x2 мощность/доход, x2 энергия (дешевле)
	BuffPowerBoostEf = "power_boost_eff" // x2 мощность/доход, x0.5 энергия (дороже)
)

// BuffDuration — один вариант длительности покупки баффа.
type BuffDuration struct {
	Key   string  // "24h" / "3d" / "7d" — приходит с фронта в поле key
	Hours float64 // длительность в часах
	Price float64 // цена в $
}

// Каталог баффов с их длительностями и ценами. Меняй Price свободно.
var BuffCatalog = map[string][]BuffDuration{
	BuffNoSleep: {
		{Key: "24h", Hours: 24, Price: 200},
		{Key: "3d", Hours: 72, Price: 400},
		{Key: "7d", Hours: 168, Price: 600},
	},
	BuffProfitBoost: {
		{Key: "24h", Hours: 24, Price: 150},
		{Key: "3d", Hours: 72, Price: 250},
		{Key: "7d", Hours: 168, Price: 450},
	},
	BuffPowerBoost: { // дешевле профит-бустера, но жрёт x2 энергии
		{Key: "24h", Hours: 24, Price: 75},
		{Key: "3d", Hours: 72, Price: 150},
		{Key: "7d", Hours: 168, Price: 230},
	},
	BuffPowerBoostEf: { // самый дорогой — x2 мощности и экономия энергии
		{Key: "24h", Hours: 24, Price: 200},
		{Key: "3d", Hours: 72, Price: 320},
		{Key: "7d", Hours: 168, Price: 620},
	},
}

// EnergyPackDef — разовая покупка энергии без длительности.
type EnergyPackDef struct {
	ID     string
	Amount float64
	Price  float64
}

// Три пакета энергии, как ты просил.
var EnergyPacks = []EnergyPackDef{
	{ID: "small", Amount: 100, Price: 25},
	{ID: "medium", Amount: 300, Price: 70},
	{ID: "large", Amount: 800, Price: 150},
}

func GetEnergyPack(id string) (EnergyPackDef, bool) {
	for _, p := range EnergyPacks {
		if p.ID == id {
			return p, true
		}
	}
	return EnergyPackDef{}, false
}
