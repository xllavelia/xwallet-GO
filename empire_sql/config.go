package empire_sql

// ============================================================
// EMPIRE — единственный файл с игровым балансом.
// Все цены, множители, лимиты и параметры — здесь.
// Логика state.go / actions.go этими значениями только
// пользуется, сама ничего не зашивает.
//
// Деньги: Empire работает на основном балансе кошелька
// (таблица wallets) — доход начисляется на него же,
// покупки списываются с него же.
// ============================================================

import "math"

// ============================================================
// ГЛОБАЛЬНЫЕ КОНСТАНТЫ
// ============================================================

// ---------- Улучшения объектов ----------
// У каждого объекта ровно 15 уровней.
const GlobalMaxLevel = 15

// ---------- Продажа ----------
// SellBackRate — какую долю от СУММЫ ВЛОЖЕНИЙ (покупка + все
// апгрейды) возвращает продажа объекта.
const SellBackRate = 0.65

// ---------- Работники ----------
// Найм одного работника стоит BaseCost объекта * эту ставку.
const WorkerHireCostRate = 0.08

// Доходный бонус от работников:
// workerMult = 1 + WorkerIncomeBonusPerWorker * (hired * quality).
// quality — среднее качество экипажа (0.4 .. ~1.5), см. workerQuality().
const WorkerIncomeBonusPerWorker = 0.06

// Опыт работника растёт по 1 единице за каждый отработанный час.
// productivity = 1 + min(WorkerMaxExpBonus, workerXp * WorkerExpRate).
const WorkerExpRate = 0.0025
const WorkerMaxExpBonus = 0.5

// Удовлетворённость (0..100) дрейфует к целевой на каждом тике:
// target = clamp(100 - expenses/income*100, WorkerSatisfactionMin, 100).
// Чем жирнее маржа объекта — тем счастливее экипаж.
const WorkerSatisfactionMin = 40.0

// Скорость сближения satisfaction с целевой (доля в час).
const WorkerSatisfactionDrift = 0.1

// ---------- Уровень империи ----------
// Бонус дохода за каждый уровень империи (кроме 1-го).
const LevelIncomeBonusPerLevel = 0.01

// XP за чистый доход: 1 XP за каждые $50 начисленного дохода.
const XpPerIncome = 1.0 / 50

// XP за траты: 1 XP за каждые $25, потраченные на покупку,
// апгрейд, найм или исследование.
const XpPerSpent = 1.0 / 25

// XP за потраченные на исследование деньги (мягче обычных трат).
const XpPerResearchSpent = 1.0 / 50

// EmpireXpForLevel — сколько XP нужно для перехода
// level -> level+1. Рост кривой: 100 * level^1.5.
func EmpireXpForLevel(level int) float64 {
	return math.Round(100 * math.Pow(float64(level), 1.5))
}

// ---------- Офлайн-тик ----------
// Базовый лимит: офлайн-доход начисляется максимум за столько
// часов с момента последнего тика. Исследование Security
// увеличивает лимит (см. EmpireResearch).
const OfflineCapHours = 8.0

// ---------- Биржа активов ----------
// Цена каждой акции пересчитывается раз в AssetPriceTickMinutes
// (random walk с сервера, одинаковый для всех игроков).
const AssetPriceTickMinutes = 5.0

// Цена не может упасть ниже AssetMinPrice.
const AssetMinPrice = 1.0

// Сколько последних точек истории цены храним (288 = сутки
// при тике раз в 5 минут).
const AssetHistoryLimit = 288

// Максимум догоняющих шагов за один вызов (сервер был выключен).
const AssetMaxCatchupSteps = 500

// Комиссия биржи при продаже акций (1%).
const AssetTradeFeeRate = 0.01

// ---------- Контракты ----------
// Одновременно доступно не больше EmpireMaxOffers офферов.
const EmpireMaxOffers = 3

// Новый оффер появляется раз в EmpireContractRegenHours часов
// (если есть свободный слот).
const EmpireContractRegenHours = 4.0

// ---------- Престиж ----------
// Престиж доступен с уровня империи PrestigeMinLevel.
const PrestigeMinLevel = 25

// Очки престижа за забег:
// PP = floor(sqrt(totalEarned / PrestigePPDivisor))
//    + floor((level - PrestigeMinLevel) / PrestigePPPerLevelEvery)
//    + PrestigeBasePP
// Например: заработал $1 000 000 -> sqrt(10) = 3 PP.
const PrestigePPDivisor = 100000.0
const PrestigePPPerLevelEvery = 5
const PrestigeBasePP = 1

// Постоянный бонус дохода за каждый PP.
const PrestigeIncomeBonusPerPP = 0.03

// ---------- Аналитика ----------
// Сколько дней дневной статистики отдаём в state.
const DailyStatsLimit = 14

// ============================================================
// СЕКТОРА
// ============================================================

type EmpireSectorDef struct {
	ID          string // машинный ключ
	Name        string // отображаемое имя (EN)
	Description string // короткое описание для карты (EN)
	Color       string // акцентный цвет сектора (подсказка для фронта)
}

var EmpireSectors = []EmpireSectorDef{
	{ID: "financial", Name: "Financial District", Description: "Banks, funds and payment infrastructure.", Color: "#4da3ff"},
	{ID: "industrial", Name: "Industrial Sector", Description: "Factories and manufacturing plants.", Color: "#ff9a4d"},
	{ID: "energy", Name: "Energy Sector", Description: "Power generation from solar to fusion.", Color: "#ffd94d"},
	{ID: "trade", Name: "Trade Sector", Description: "Retail chains and marketplaces.", Color: "#6fe3a5"},
	{ID: "tech", Name: "Tech Sector", Description: "Data centers, clouds and quantum labs.", Color: "#b98cff"},
	{ID: "luxury", Name: "Luxury Sector", Description: "Hotels, casinos and resorts.", Color: "#ff6f91"},
	{ID: "research", Name: "Research Sector", Description: "Labs and institutes generating Research Points.", Color: "#5ee6e6"},
}

// Порядок редкости (для сортировок и проверок контрактов).
var EmpireRarityOrder = map[string]int{
	"common": 1, "uncommon": 2, "rare": 3, "epic": 4, "legendary": 5,
}

// ============================================================
// ОБЪЕКТЫ ИМПЕРИИ (43 штуки)
// ============================================================
// BaseIncome / BaseExpenses — валовый доход и расходы в ЧАС
// на 1 уровне. На уровне L:
//   доход   = BaseIncome  * IncomeMult^(L-1)
//   расходы = BaseExpenses * ExpenseMult^(L-1)
//   цена апгрейда L -> L+1 = UpgradeCostBase * UpgradeCostMult^(L-1)
//   базовая стоимость = BaseCost * ValueMult^(L-1)
// ResearchPerHour — RP/час (только research-сектор, иначе 0).
// UnlockLevel — минимальный уровень империи для покупки.

type EmpireObjectDef struct {
	ID              string
	Name            string
	Sector          string
	Rarity          string
	Description     string
	BaseCost        float64
	BaseIncome      float64
	BaseExpenses    float64
	WorkerSlots     int
	ResearchPerHour float64
	UnlockLevel     int
	UpgradeCostBase float64
	UpgradeCostMult float64
	IncomeMult      float64
	ExpenseMult     float64
	ValueMult       float64
}

func (d EmpireObjectDef) IncomeAt(level int) float64 {
	return d.BaseIncome * math.Pow(d.IncomeMult, float64(level-1))
}

func (d EmpireObjectDef) ExpensesAt(level int) float64 {
	return d.BaseExpenses * math.Pow(d.ExpenseMult, float64(level-1))
}

func (d EmpireObjectDef) UpgradeCost(level int) float64 {
	if level >= GlobalMaxLevel {
		return 0
	}
	return d.UpgradeCostBase * math.Pow(d.UpgradeCostMult, float64(level-1))
}

func (d EmpireObjectDef) BaseValueAt(level int) float64 {
	return d.BaseCost * math.Pow(d.ValueMult, float64(level-1))
}

var EmpireObjects = []EmpireObjectDef{
	// ---------- Financial District ----------
	{
		ID: "payment_terminal", Name: "Payment Terminal", Sector: "financial", Rarity: "common",
		Description: "Compact terminal processing local card payments.",
		BaseCost:    50, BaseIncome: 5, BaseExpenses: 2, WorkerSlots: 1, ResearchPerHour: 0, UnlockLevel: 1,
		UpgradeCostBase: 25, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "atm_network", Name: "ATM Network", Sector: "financial", Rarity: "common",
		Description: "A small network of cash machines earning fees.",
		BaseCost:    120, BaseIncome: 5, BaseExpenses: 5, WorkerSlots: 1, ResearchPerHour: 0, UnlockLevel: 1,
		UpgradeCostBase: 60, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "exchange_office", Name: "Exchange Office", Sector: "financial", Rarity: "uncommon",
		Description: "Currency exchange booth with a steady spread.",
		BaseCost:    240, BaseIncome: 10, BaseExpenses: 10, WorkerSlots: 2, ResearchPerHour: 0, UnlockLevel: 2,
		UpgradeCostBase: 120, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "bank_branch", Name: "Bank Branch", Sector: "financial", Rarity: "uncommon",
		Description: "Classic branch with deposits, loans and fees.",
		BaseCost:    500, BaseIncome: 24, BaseExpenses: 20, WorkerSlots: 3, ResearchPerHour: 0, UnlockLevel: 3,
		UpgradeCostBase: 150, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "investment_fund", Name: "Investment Fund", Sector: "financial", Rarity: "rare",
		Description: "Managed fund earning management fees.",
		BaseCost:    750, BaseIncome: 28, BaseExpenses: 52, WorkerSlots: 3, ResearchPerHour: 0, UnlockLevel: 5,
		UpgradeCostBase: 200, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "digital_bank", Name: "Digital Bank", Sector: "financial", Rarity: "epic",
		Description: "Mobile-first bank with millions of users.",
		BaseCost:    1000, BaseIncome: 34, BaseExpenses: 118, WorkerSlots: 5, ResearchPerHour: 0, UnlockLevel: 8,
		UpgradeCostBase: 320, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "private_bank", Name: "Private Bank", Sector: "financial", Rarity: "legendary",
		Description: "Private banking for high net worth clients.",
		BaseCost:    1400, BaseIncome: 42, BaseExpenses: 260, WorkerSlots: 6, ResearchPerHour: 0, UnlockLevel: 12,
		UpgradeCostBase: 450, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},

	{
		ID: "workshop", Name: "Workshop", Sector: "industrial", Rarity: "common",
		Description: "Small workshop producing goods by hand.",
		BaseCost:    40, BaseIncome: 2, BaseExpenses: 5, WorkerSlots: 2, ResearchPerHour: 0, UnlockLevel: 1,
		UpgradeCostBase: 12, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "craft_studio", Name: "Craft Studio", Sector: "industrial", Rarity: "common",
		Description: "Artisan studio with premium handmade goods.",
		BaseCost:    80, BaseIncome: 5, BaseExpenses: 10, WorkerSlots: 2, ResearchPerHour: 0, UnlockLevel: 1,
		UpgradeCostBase: 24, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "factory", Name: "Factory", Sector: "industrial", Rarity: "uncommon",
		Description: "Assembly line manufacturing at scale.",
		BaseCost:    160, BaseIncome: 12, BaseExpenses: 23, WorkerSlots: 4, ResearchPerHour: 0, UnlockLevel: 2,
		UpgradeCostBase: 48, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "manufacturing_plant", Name: "Manufacturing Plant", Sector: "industrial", Rarity: "uncommon",
		Description: "Modern plant with robotic assistance.",
		BaseCost:    250, BaseIncome: 16, BaseExpenses: 50, WorkerSlots: 5, ResearchPerHour: 0, UnlockLevel: 3,
		UpgradeCostBase: 75, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "mega_factory", Name: "Mega Factory", Sector: "industrial", Rarity: "rare",
		Description: "Huge facility producing thousands of units hourly.",
		BaseCost:    300, BaseIncome: 18, BaseExpenses: 112, WorkerSlots: 8, ResearchPerHour: 0, UnlockLevel: 5,
		UpgradeCostBase: 100, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "industrial_complex", Name: "Industrial Complex", Sector: "industrial", Rarity: "epic",
		Description: "An entire industrial park under your control.",
		BaseCost:    620, BaseIncome: 30, BaseExpenses: 245, WorkerSlots: 10, ResearchPerHour: 0, UnlockLevel: 8,
		UpgradeCostBase: 220, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},

	// ---------- Energy Sector ----------
	{
		ID: "solar_farm", Name: "Solar Farm", Sector: "energy", Rarity: "common",
		Description: "Solar panels converting sunlight into steady income.",
		BaseCost:    50, BaseIncome: 4, BaseExpenses: 6, WorkerSlots: 1, ResearchPerHour: 0, UnlockLevel: 1,
		UpgradeCostBase: 20, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "wind_farm", Name: "Wind Farm", Sector: "energy", Rarity: "common",
		Description: "Turbines on a windy ridge selling clean power.",
		BaseCost:    120, BaseIncome: 8, BaseExpenses: 13, WorkerSlots: 2, ResearchPerHour: 0, UnlockLevel: 1,
		UpgradeCostBase: 45, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "hydro_plant", Name: "Hydro Plant", Sector: "energy", Rarity: "uncommon",
		Description: "Hydroelectric station on the river.",
		BaseCost:    180, BaseIncome: 10, BaseExpenses: 30, WorkerSlots: 3, ResearchPerHour: 0, UnlockLevel: 2,
		UpgradeCostBase: 55, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "power_station", Name: "Power Station", Sector: "energy", Rarity: "rare",
		Description: "Central station supplying the whole district.",
		BaseCost:    240, BaseIncome: 14, BaseExpenses: 70, WorkerSlots: 5, ResearchPerHour: 0, UnlockLevel: 4,
		UpgradeCostBase: 75, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "nuclear_plant", Name: "Nuclear Plant", Sector: "energy", Rarity: "epic",
		Description: "Nuclear power with massive output.",
		BaseCost:    500, BaseIncome: 21, BaseExpenses: 160, WorkerSlots: 7, ResearchPerHour: 0, UnlockLevel: 7,
		UpgradeCostBase: 120, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "fusion_plant", Name: "Fusion Plant", Sector: "energy", Rarity: "legendary",
		Description: "Fusion reactor — the future of energy.",
		BaseCost:    1000, BaseIncome: 38, BaseExpenses: 360, WorkerSlots: 8, ResearchPerHour: 0, UnlockLevel: 11,
		UpgradeCostBase: 200, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},

	// ---------- Trade Sector ----------
	{
		ID: "mini_market", Name: "Mini Market", Sector: "trade", Rarity: "common",
		Description: "Neighborhood store with daily regulars.",
		BaseCost:    30, BaseIncome: 2, BaseExpenses: 4, WorkerSlots: 1, ResearchPerHour: 0, UnlockLevel: 1,
		UpgradeCostBase: 6, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "store", Name: "Store", Sector: "trade", Rarity: "common",
		Description: "Street-level retail with a wide assortment.",
		BaseCost:    70, BaseIncome: 5, BaseExpenses: 8, WorkerSlots: 2, ResearchPerHour: 0, UnlockLevel: 1,
		UpgradeCostBase: 14, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "supermarket", Name: "Supermarket", Sector: "trade", Rarity: "uncommon",
		Description: "Full-size supermarket with high turnover.",
		BaseCost:    170, BaseIncome: 10, BaseExpenses: 19, WorkerSlots: 4, ResearchPerHour: 0, UnlockLevel: 2,
		UpgradeCostBase: 30, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "shopping_center", Name: "Shopping Center", Sector: "trade", Rarity: "uncommon",
		Description: "A center hosting dozens of retailers.",
		BaseCost:    400, BaseIncome: 22, BaseExpenses: 43, WorkerSlots: 6, ResearchPerHour: 0, UnlockLevel: 3,
		UpgradeCostBase: 60, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "mall", Name: "Mall", Sector: "trade", Rarity: "rare",
		Description: "City mall with anchor stores and food court.",
		BaseCost:    600, BaseIncome: 28, BaseExpenses: 98, WorkerSlots: 8, ResearchPerHour: 0, UnlockLevel: 5,
		UpgradeCostBase: 90, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "global_marketplace", Name: "Global Marketplace", Sector: "trade", Rarity: "epic",
		Description: "Online marketplace shipping worldwide.",
		BaseCost:    900, BaseIncome: 34, BaseExpenses: 230, WorkerSlots: 10, ResearchPerHour: 0, UnlockLevel: 9,
		UpgradeCostBase: 120, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},

	// ---------- Tech Sector ----------
	{
		ID: "server_rack", Name: "Server Rack", Sector: "tech", Rarity: "common",
		Description: "A rented rack hosting services.",
		BaseCost:    100, BaseIncome: 4, BaseExpenses: 12, WorkerSlots: 1, ResearchPerHour: 0, UnlockLevel: 1,
		UpgradeCostBase: 26, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "data_center", Name: "Data Center", Sector: "tech", Rarity: "uncommon",
		Description: "Facility selling compute and storage.",
		BaseCost:    500, BaseIncome: 24, BaseExpenses: 28, WorkerSlots: 3, ResearchPerHour: 0, UnlockLevel: 2,
		UpgradeCostBase: 50, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "cloud_cluster", Name: "Cloud Cluster", Sector: "tech", Rarity: "rare",
		Description: "Elastic cloud infrastructure on demand.",
		BaseCost:    700, BaseIncome: 30, BaseExpenses: 66, WorkerSlots: 4, ResearchPerHour: 0, UnlockLevel: 4,
		UpgradeCostBase: 70, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "ai_facility", Name: "AI Facility", Sector: "tech", Rarity: "epic",
		Description: "GPU cluster training and renting AI models.",
		BaseCost:    1000, BaseIncome: 40, BaseExpenses: 175, WorkerSlots: 6, ResearchPerHour: 0, UnlockLevel: 7,
		UpgradeCostBase: 120, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "quantum_lab", Name: "Quantum Lab", Sector: "tech", Rarity: "legendary",
		Description: "Research lab selling quantum computing access.",
		BaseCost:    1200, BaseIncome: 44, BaseExpenses: 470, WorkerSlots: 7, ResearchPerHour: 0, UnlockLevel: 11,
		UpgradeCostBase: 150, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "satellite_network", Name: "Satellite Network", Sector: "tech", Rarity: "legendary",
		Description: "Constellation of satellites selling global connectivity.",
		BaseCost:    1500, BaseIncome: 50, BaseExpenses: 1200, WorkerSlots: 8, ResearchPerHour: 0, UnlockLevel: 15,
		UpgradeCostBase: 170, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},

	// ---------- Luxury Sector ----------
	{
		ID: "restaurant", Name: "Restaurant", Sector: "luxury", Rarity: "common",
		Description: "Cozy restaurant with a loyal clientele.",
		BaseCost:    200, BaseIncome: 10, BaseExpenses: 14, WorkerSlots: 2, ResearchPerHour: 0, UnlockLevel: 1,
		UpgradeCostBase: 40, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "hotel", Name: "Hotel", Sector: "luxury", Rarity: "uncommon",
		Description: "City hotel with high occupancy.",
		BaseCost:    300, BaseIncome: 14, BaseExpenses: 36, WorkerSlots: 4, ResearchPerHour: 0, UnlockLevel: 2,
		UpgradeCostBase: 1500, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "casino", Name: "Casino", Sector: "luxury", Rarity: "rare",
		Description: "The house always wins.",
		BaseCost:    700, BaseIncome: 28, BaseExpenses: 115, WorkerSlots: 5, ResearchPerHour: 0, UnlockLevel: 4,
		UpgradeCostBase: 4800, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "resort", Name: "Resort", Sector: "luxury", Rarity: "epic",
		Description: "Beach resort with premium pricing.",
		BaseCost:    900, BaseIncome: 34, BaseExpenses: 295, WorkerSlots: 7, ResearchPerHour: 0, UnlockLevel: 7,
		UpgradeCostBase: 132, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "private_island", Name: "Private Island", Sector: "luxury", Rarity: "legendary",
		Description: "Your own island — hotel, marina, helipad.",
		BaseCost:    1600, BaseIncome: 50, BaseExpenses: 740, WorkerSlots: 8, ResearchPerHour: 0, UnlockLevel: 11,
		UpgradeCostBase: 360, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "space_tourism", Name: "Space Tourism Center", Sector: "luxury", Rarity: "legendary",
		Description: "Suborbital flights for the ultra-wealthy.",
		BaseCost:    2500, BaseIncome: 80, BaseExpenses: 1800, WorkerSlots: 9, ResearchPerHour: 0, UnlockLevel: 16,
		UpgradeCostBase: 500, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},

	// ---------- Research Sector ----------
	// Объекты research-сектора дают ResearchPerHour (RP/час),
	// который тратится на исследования (см. EmpireResearch).
	{
		ID: "research_lab", Name: "Research Lab", Sector: "research", Rarity: "uncommon",
		Description: "Basic lab generating research points.",
		BaseCost:    150, BaseIncome: 10, BaseExpenses: 30, WorkerSlots: 2, ResearchPerHour: 2, UnlockLevel: 2,
		UpgradeCostBase: 15, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "science_institute", Name: "Science Institute", Sector: "research", Rarity: "rare",
		Description: "Institute running multiple research programs.",
		BaseCost:    300, BaseIncome: 20, BaseExpenses: 72, WorkerSlots: 4, ResearchPerHour: 4, UnlockLevel: 3,
		UpgradeCostBase: 30, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "rnd_center", Name: "R&D Center", Sector: "research", Rarity: "rare",
		Description: "Corporate R&D with applied science focus.",
		BaseCost:    600, BaseIncome: 18, BaseExpenses: 175, WorkerSlots: 5, ResearchPerHour: 7, UnlockLevel: 5,
		UpgradeCostBase: 60, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "innovation_hub", Name: "Innovation Hub", Sector: "research", Rarity: "epic",
		Description: "Hub spinning up startups and patents.",
		BaseCost:    900, BaseIncome: 28, BaseExpenses: 420, WorkerSlots: 7, ResearchPerHour: 12, UnlockLevel: 8,
		UpgradeCostBase: 90, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "think_tank", Name: "Think Tank", Sector: "research", Rarity: "epic",
		Description: "Elite analysts generating breakthrough ideas.",
		BaseCost:    1200, BaseIncome: 32, BaseExpenses: 480, WorkerSlots: 6, ResearchPerHour: 15, UnlockLevel: 9,
		UpgradeCostBase: 120, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
	{
		ID: "university", Name: "University", Sector: "research", Rarity: "legendary",
		Description: "Your own university — research at industrial scale.",
		BaseCost:    1500, BaseIncome: 38, BaseExpenses: 1000, WorkerSlots: 9, ResearchPerHour: 25, UnlockLevel: 13,
		UpgradeCostBase: 150, UpgradeCostMult: 1.55, IncomeMult: 1.22, ExpenseMult: 1.07, ValueMult: 1.18,
	},
}

var EmpireObjectByID = map[string]EmpireObjectDef{}

// ============================================================
// ИССЛЕДОВАНИЯ (6 направлений, по 10 уровней)
// Эффект направления складывается с каждым уровнем:
//   automation: +EffectPerLevel ко всему доходу объектов
//   ai:         +EffectPerLevel к бонусу работников
//   energy:     -EffectPerLevel ко всем расходам
//   finance:    +EffectPerLevel к дивидендам акций
//   trading:    +EffectPerLevel к денежным наградам контрактов
//   security:   +EffectPerLevel к лимиту офлайн-начисления
// Цена уровня L -> L+1:
//   деньги = MoneyBase * MoneyMult^L   (L от 0)
//   RP     = RPBase * RPMult^L
// ============================================================

type EmpireResearchDef struct {
	ID             string
	Name           string
	Description    string // описание эффекта (EN, для UI)
	EffectKind     string // см. ComputeBonuses
	EffectPerLevel float64
	MaxLevel       int
	MoneyBase      float64
	MoneyMult      float64
	RPBase         float64
	RPMult         float64
}

var EmpireResearch = []EmpireResearchDef{
	{
		ID: "automation", Name: "Automation",
		Description: "+3% income of all facilities per level.",
		EffectKind:  "object_income", EffectPerLevel: 0.03, MaxLevel: 10,
		MoneyBase: 100, MoneyMult: 1.65, RPBase: 40, RPMult: 1.6,
	},
	{
		ID: "ai", Name: "Artificial Intelligence",
		Description: "+4% worker productivity bonus per level.",
		EffectKind:  "worker_bonus", EffectPerLevel: 0.04, MaxLevel: 10,
		MoneyBase: 100, MoneyMult: 1.65, RPBase: 40, RPMult: 1.6,
	},
	{
		ID: "energy", Name: "Energy Efficiency",
		Description: "-3% all facility expenses per level.",
		EffectKind:  "expenses_cut", EffectPerLevel: 0.03, MaxLevel: 10,
		MoneyBase: 100, MoneyMult: 1.65, RPBase: 40, RPMult: 1.6,
	},
	{
		ID: "finance", Name: "Financial Engineering",
		Description: "+5% asset dividends per level.",
		EffectKind:  "dividends", EffectPerLevel: 0.05, MaxLevel: 10,
		MoneyBase: 110, MoneyMult: 1.65, RPBase: 40, RPMult: 1.6,
	},
	{
		ID: "trading", Name: "Trading",
		Description: "+4% contract money rewards per level.",
		EffectKind:  "contract_reward", EffectPerLevel: 0.04, MaxLevel: 10,
		MoneyBase: 100, MoneyMult: 1.65, RPBase: 40, RPMult: 1.6,
	},
	{
		ID: "security", Name: "Security",
		Description: "+10% offline earnings cap per level.",
		EffectKind:  "offline_cap", EffectPerLevel: 0.10, MaxLevel: 10,
		MoneyBase: 150, MoneyMult: 1.65, RPBase: 40, RPMult: 1.6,
	},
}

func (d EmpireResearchDef) MoneyCost(currentLevel int) float64 {
	return d.MoneyBase * math.Pow(d.MoneyMult, float64(currentLevel))
}

func (d EmpireResearchDef) RPCost(currentLevel int) float64 {
	return math.Round(d.RPBase * math.Pow(d.RPMult, float64(currentLevel)))
}

var EmpireResearchByID = map[string]EmpireResearchDef{}

// ============================================================
// БИРЖА АКТИВОВ (общие цены для всего сервера)
// Цена ходит random walk'ом: каждый тик
// change = price * (rand*2-1) * Volatility.
// Дивиденды начисляются ежечасно:
// shares * price * DailyDividendYield / 24 за каждый час.
// ============================================================

type EmpireAssetDef struct {
	ID                 string
	Name               string
	Description        string
	BasePrice          float64 // стартовая цена (при сидировании)
	Volatility         float64 // амплитуда колебаний за тик (доля)
	DailyDividendYield float64 // дневная доходность дивидендов (доля)
}

var EmpireAssets = []EmpireAssetDef{
	{
		ID: "tech_shares", Name: "Tech Shares",
		Description: "Shares of leading technology companies.",
		BasePrice:   100, Volatility: 0.04, DailyDividendYield: 0.012,
	},
	{
		ID: "energy_shares", Name: "Energy Shares",
		Description: "Shares of energy producers and utilities.",
		BasePrice:   80, Volatility: 0.03, DailyDividendYield: 0.016,
	},
	{
		ID: "finance_shares", Name: "Finance Shares",
		Description: "Shares of banks and financial groups.",
		BasePrice:   120, Volatility: 0.025, DailyDividendYield: 0.010,
	},
	{
		ID: "industrial_shares", Name: "Industrial Shares",
		Description: "Shares of industrial and manufacturing giants.",
		BasePrice:   60, Volatility: 0.035, DailyDividendYield: 0.014,
	},
}

var EmpireAssetByID = map[string]EmpireAssetDef{}

// ============================================================
// КОНТРАКТЫ (шаблоны авто-генерируемых миссий)
// Target контракта = BaseTarget + TargetPerLevel * (empireLevel-1).
// Награды масштабируются уровнем империи аналогично.
// Metric — что копит прогресс:
//   revenue        — весь начисленный доход империи
//   energy_income  — доход объектов energy-сектора
//   objects_bought — количество купленных объектов
//   facility_rare  — покупка объекта редкости Rare+ (1 за контракт)
// ============================================================

type EmpireContractTemplate struct {
	ID             string
	Name           string
	Description    string
	Metric         string
	BaseTarget     float64
	TargetPerLevel float64
	DurationHours  float64
	MoneyBase      float64
	MoneyPerLevel  float64
	XpBase         float64
	XpPerLevel     float64
	RpBase         float64
	RpPerLevel     float64
}

var EmpireContractTemplates = []EmpireContractTemplate{
	{
		ID: "generate_revenue", Name: "Generate Revenue",
		Description: "Earn the target amount of empire income.",
		Metric:      "revenue",
		BaseTarget:  500, TargetPerLevel: 350, DurationHours: 24,
		MoneyBase: 150, MoneyPerLevel: 55, XpBase: 25, XpPerLevel: 8, RpBase: 8, RpPerLevel: 2,
	},
	{
		ID: "expand_empire", Name: "Expand Empire",
		Description: "Purchase new facilities for your empire.",
		Metric:      "objects_bought",
		BaseTarget:  1, TargetPerLevel: 0.25, DurationHours: 24,
		MoneyBase: 200, MoneyPerLevel: 70, XpBase: 30, XpPerLevel: 10, RpBase: 8, RpPerLevel: 2,
	},
	{
		ID: "deliver_energy", Name: "Deliver Energy",
		Description: "Earn the target amount of income from the Energy sector.",
		Metric:      "energy_income",
		BaseTarget:  300, TargetPerLevel: 250, DurationHours: 18,
		MoneyBase: 120, MoneyPerLevel: 45, XpBase: 25, XpPerLevel: 8, RpBase: 8, RpPerLevel: 2,
	},
	{
		ID: "open_facility", Name: "Open New Facility",
		Description: "Open a facility of Rare rarity or higher.",
		Metric:      "facility_rare",
		BaseTarget:  1, TargetPerLevel: 0, DurationHours: 36,
		MoneyBase: 250, MoneyPerLevel: 80, XpBase: 40, XpPerLevel: 12, RpBase: 10, RpPerLevel: 2,
	},
}

// ============================================================
// БУСТЕРЫ
// BoosterPrices[тип][длительность в часах] = цена в $.
// Повторная покупка продлевает действие.
// ============================================================

type EmpireBoosterDef struct {
	ID          string
	Name        string
	Description string
}

var EmpireBoosters = []EmpireBoosterDef{
	{ID: "income_x2", Name: "2x Income", Description: "Doubles all facility income."},
	{ID: "research_x2", Name: "2x Research", Description: "Doubles Research Points generation."},
	{ID: "workers_x2", Name: "2x Productivity", Description: "Doubles worker productivity bonus."},
	{ID: "expenses_half", Name: "-50% Expenses", Description: "Halves all facility expenses."},
}

var BoosterDurations = []int{1, 24, 72, 168} // 1ч, сутки, 3 дня, неделя

var BoosterPrices = map[string]map[int]float64{
	"income_x2":     {1: 20, 24: 120, 72: 2800, 168: 550},
	"research_x2":   {1: 15, 24: 90, 72: 2200, 168: 420},
	"workers_x2":    {1: 15, 24: 90, 72: 2200, 168: 420},
	"expenses_half": {1: 18, 24: 110, 72: 2600, 168: 500},
}

func init() {
	for _, d := range EmpireObjects {
		EmpireObjectByID[d.ID] = d
	}
	for _, d := range EmpireResearch {
		EmpireResearchByID[d.ID] = d
	}
	for _, d := range EmpireAssets {
		EmpireAssetByID[d.ID] = d
	}
}
