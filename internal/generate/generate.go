// Пакет generate: частотно-взвешенная генерация билетов.
package generate

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"sort"
)

// DrawFreq — вход частот: одна комбинация прошедшего розыгрыша.
type DrawFreq struct {
	Numbers []int
	Bonus   int
}

// Ticket — сгенерированный билет.
type Ticket struct {
	Numbers []int `json:"numbers"` // 7 чисел 1–35 по возрастанию
	Bonus   int   `json:"bonus"`   // 1–54
}

// RNG — источник случайности; изолирован для детерминированных тестов.
type RNG interface{ Intn(n int) int }

// CryptoRand — RNG на crypto/rand.
type CryptoRand struct{}

func (CryptoRand) Intn(n int) int {
	if n <= 0 {
		panic(fmt.Sprintf("Intn: недопустимое n=%d", n))
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		panic(err)
	}
	return int(v.Int64())
}

// Frequencies считает, сколько раз каждое число выпадало в прошлых розыгрышах.
// main: индекс 0..34 ↔ числа 1..35; bonus: 0..53 ↔ 1..54.
func Frequencies(draws []DrawFreq) (main, bonus []int) {
	main = make([]int, 35)
	bonus = make([]int, 54)
	for _, d := range draws {
		for _, n := range d.Numbers {
			if n >= 1 && n <= 35 {
				main[n-1]++
			}
		}
		if d.Bonus >= 1 && d.Bonus <= 54 {
			bonus[d.Bonus-1]++
		}
	}
	return main, bonus
}

// weightedPick выбирает число (индекс+1) с вероятностью ∝ (вес+1).
// taken может быть nil (ничего не исключено).
func weightedPick(weights []int, taken []bool, rng RNG) int {
	total := 0
	for i, w := range weights {
		if taken != nil && taken[i] {
			continue
		}
		total += w + 1 // +1 — сглаживание: «холодные» числа остаются возможными
	}
	r := rng.Intn(total)
	cum := 0
	for i, w := range weights {
		if taken != nil && taken[i] {
			continue
		}
		cum += w + 1
		if r < cum {
			return i + 1
		}
	}
	panic("weightedPick: недостижимо")
}

// GenerateTicket генерирует один билет: 7 разных чисел без повторов + бонус.
// Предусловия: len(mainFreq) >= 7, len(bonusFreq) >= 1, все веса >= 0
// (при нарушении — паника из weightedPick/Intn; в приложении частоты всегда 35/54).
func GenerateTicket(mainFreq, bonusFreq []int, rng RNG) Ticket {
	taken := make([]bool, len(mainFreq))
	nums := make([]int, 0, 7)
	for len(nums) < 7 {
		n := weightedPick(mainFreq, taken, rng)
		taken[n-1] = true
		nums = append(nums, n)
	}
	sort.Ints(nums)
	return Ticket{Numbers: nums, Bonus: weightedPick(bonusFreq, nil, rng)}
}

// dupAttempts — максимум попыток на один билет; последняя может принять дубликат.
const dupAttempts = 20

// GenerateBatch генерирует count билетов, стараясь не повторяться внутри пачки.
// count должен быть >= 0; диапазон 1..100 проверяет вызывающий.
func GenerateBatch(mainFreq, bonusFreq []int, count int, rng RNG) []Ticket {
	seen := make(map[string]struct{}, count)
	tickets := make([]Ticket, 0, count)
	for len(tickets) < count {
		var t Ticket
		for attempt := 0; ; attempt++ {
			t = GenerateTicket(mainFreq, bonusFreq, rng)
			key := Key(t)
			if _, dup := seen[key]; !dup || attempt >= dupAttempts-1 {
				seen[key] = struct{}{}
				tickets = append(tickets, t)
				break
			}
		}
	}
	return tickets
}

// Key — строковый ключ билета для дедупликации пачки (числа + бонус).
// Экспортирован: используется также пакетом llm.
func Key(t Ticket) string {
	return fmt.Sprintf("%v+%d", t.Numbers, t.Bonus)
}
