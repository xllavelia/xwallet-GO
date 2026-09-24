package pixel_sql

// ── Вот та самая строка "одна клетка = 5%" — меняй здесь ──
var PercentPerCell = 1.0

// ── Размер поля — меняешь любое из двух чисел, фронт подхватит сам ──
var GridCols = 7 // "семь в ширину"
var GridRows = 6 // "шесть в длину"

// ── Доля заминированных клеток = 1 / MineFractionDenominator ──
var MineFractionDenominator = 3

var LivesCount = 2
var BettingDuration = 15 // секунд на перерыв/ставки
var RoundDuration = 30   // секунд максимум на партию
var RevealDuration = 3   // секунд на просмотр поля после поражения

func TotalCells() int { return GridCols * GridRows }
func MineCount() int  { return TotalCells() / MineFractionDenominator }
