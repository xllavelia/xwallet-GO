package slots_sql

// ============================================================
// КОНФИГ SLOTS
// Все игровые значения модуля правятся только в этом файле:
// символы, таблица выплат, уровни спина, наборы спинов,
// редкие события и лимиты. Комментарии — прямо у значений.
// ============================================================

// ---------- Символы барабанов ----------
// Weight — относительная частота (чем больше, тем чаще выпадает).
// Mult   — множитель выплаты за три таких символа на линии
//          (до умножения на payout_mult уровня).
type SlotsSymbol struct {
	ID     string
	Name   string
	Weight float64
	Mult   float64
}

var SlotsSymbols = []SlotsSymbol{
	{ID: "coin", Name: "Coin", Weight: 30, Mult: 2},
	{ID: "card", Name: "Card", Weight: 22, Mult: 3},
	{ID: "chart", Name: "Growth", Weight: 18, Mult: 4},
	{ID: "briefcase", Name: "Briefcase", Weight: 14, Mult: 6},
	{ID: "safe", Name: "Safe", Weight: 10, Mult: 8},
	{ID: "bank", Name: "Bank", Weight: 8, Mult: 10},
	{ID: "crystal", Name: "Crystal", Weight: 5, Mult: 15},
	{ID: "crown", Name: "Crown", Weight: 3, Mult: 25},
	{ID: "diamond", Name: "Diamond", Weight: 1.5, Mult: 50},
}

var SlotsSymbolByID = map[string]*SlotsSymbol{}

func init() {
	for i := range SlotsSymbols {
		SlotsSymbolByID[SlotsSymbols[i].ID] = &SlotsSymbols[i]
	}
}

// ---------- Таблица выплат ----------
// Правила с wildcard (пустая строка = любой символ) проверяются
// СВЕРХУ ВНИЗ, первое совпадение выигрывает. Три одинаковых символа
// проверяются ДО wildcard-правил: сначала точное совпадение с таблицей,
// потом fallback на Mult символа. Пары описывайте после особых сочетаний.
type SlotsCombo struct {
	Reels [3]string // "" = любой символ
	Mult  float64
}

var SlotsCombos = []SlotsCombo{
	{Reels: [3]string{"crown", "crown", "diamond"}, Mult: 30}, // особое сочетание
	{Reels: [3]string{"safe", "safe", "diamond"}, Mult: 20},   // особое сочетание
	{Reels: [3]string{"diamond", "diamond", ""}, Mult: 8},     // пара бриллиантов
	{Reels: [3]string{"crown", "crown", ""}, Mult: 6},         // пара корон
	{Reels: [3]string{"crystal", "crystal", ""}, Mult: 5},
	{Reels: [3]string{"bank", "bank", ""}, Mult: 4},
	{Reels: [3]string{"chart", "chart", ""}, Mult: 3},
}

// ---------- Уровни спина ----------
// SpinsCost  — сколько спинов списывается за одно вращение.
// PayoutMult — множитель выплаты уровня (накладывается на mult комбинации).
// Pool       — какие символы крутятся на барабанах уровня
//              (верхние уровни отрезают дешёвые символы).
type SlotsLevel struct {
	ID         string
	Name       string
	SpinsCost  int
	PayoutMult float64
	Pool       []string
}

var SlotsLevels = []SlotsLevel{
	{
		ID: "standard", Name: "Standard",
		SpinsCost: 1, PayoutMult: 1.0,
		Pool: []string{"coin", "card", "chart", "briefcase", "safe", "bank", "crystal", "crown", "diamond"},
	},
	{
		ID: "premium", Name: "Premium",
		SpinsCost: 2, PayoutMult: 2.4,
		Pool: []string{"card", "chart", "briefcase", "safe", "bank", "crystal", "crown", "diamond"},
	},
	{
		ID: "elite", Name: "Elite",
		SpinsCost: 5, PayoutMult: 7.0,
		Pool: []string{"briefcase", "safe", "bank", "crystal", "crown", "diamond"},
	},
}

// ---------- Магазин наборов спинов ----------
// Price в USDT со счёта кошелька. Spins начисляются на баланс спинов.
type SlotsPack struct {
	ID    string
	Name  string
	Spins int
	Price float64
}

var SlotsPacks = []SlotsPack{
	{ID: "starter", Name: "Starter", Spins: 20, Price: 1.0},
	{ID: "medium", Name: "Medium", Spins: 55, Price: 2.5},
	{ID: "large", Name: "Large", Spins: 120, Price: 5.0},
}

// ---------- Редкие события ----------
// Weight — вероятность за спин (0.004 = 0.4%). За один спин выпадает
// не больше одного события. Эффект обрабатывается в actions.go по ID:
//   lucky_spin    — барабаны перекручиваются до первого выигрыша
//   double_reward — перекрут до выигрыша, выплата ×2
//   bonus_round   — мгновенно начисляет BonusSpins бесплатных спинов
type SlotsEvent struct {
	ID         string
	Name       string
	Weight     float64
	BonusSpins int
}

var SlotsEvents = []SlotsEvent{
	{ID: "lucky_spin", Name: "Lucky Spin", Weight: 0.004},
	{ID: "double_reward", Name: "Double Reward", Weight: 0.0025},
	{ID: "bonus_round", Name: "Bonus Round", Weight: 0.0015, BonusSpins: 3},
}

// ---------- Прочие константы ----------
const (
	// Сколько последних спинов отдаём в state для блока истории.
	HistoryStateLimit = 15
	// Сколько строк истории храним в таблице (старые подчищаются).
	HistoryKeepLimit = 200
	// Номинал спина, когда пул ставок пуст (игра только на бонусных
	// спинах) — чтобы бонусные спины тоже могли выиграть.
	MinStakeUSDT = 0.05
	// Сколько раз максимум перекручиваем барабаны в событиях
	// lucky_spin / double_reward (страховка от бесконечного цикла).
	EventRerollMax = 100
)
