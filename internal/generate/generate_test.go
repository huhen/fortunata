package generate

import (
	"reflect"
	"testing"
)

// fakeRng выдаёт значения из очереди; после исчерпания возвращает 0.
type fakeRng struct {
	vals  []int
	calls []int
}

func (f *fakeRng) Intn(n int) int {
	f.calls = append(f.calls, n)
	if len(f.vals) == 0 {
		return 0
	}
	v := f.vals[0]
	f.vals = f.vals[1:]
	return v
}

func TestFrequencies(t *testing.T) {
	draws := []DrawFreq{
		{Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 48},
		{Numbers: []int{7, 8, 9, 10, 11, 12, 13}, Bonus: 48},
	}
	main, bonus := Frequencies(draws)
	if main[6] != 2 { // число 7 выпадало дважды
		t.Fatalf("main[6] = %d, хотим 2", main[6])
	}
	if main[0] != 1 || bonus[47] != 2 {
		t.Fatalf("main[0]=%d bonus[47]=%d", main[0], bonus[47])
	}
	if len(main) != 35 || len(bonus) != 54 {
		t.Fatalf("размеры: %d/%d", len(main), len(bonus))
	}
}

func TestGenerateTicketEmptyBase(t *testing.T) {
	// Пустая база: все веса равны, rng всё время даёт 0 → числа 1..7, бонус 1.
	rng := &fakeRng{}
	ticket := GenerateTicket(make([]int, 35), make([]int, 54), rng)
	want := []int{1, 2, 3, 4, 5, 6, 7}
	if !reflect.DeepEqual(ticket.Numbers, want) || ticket.Bonus != 1 {
		t.Fatalf("ticket = %+v", ticket)
	}
}

func TestGenerateTicketWeighted(t *testing.T) {
	// Число 1 имеет частоту 100 (вес 101), остальные — вес 1.
	mainFreq := make([]int, 35)
	mainFreq[0] = 100
	rng := &fakeRng{vals: []int{0, 32}} // после 0 → первый взвешенный; 32 из 34 → число 34
	ticket := GenerateTicket(mainFreq, make([]int, 54), rng)
	want := []int{1, 2, 3, 4, 5, 6, 34} // 1 выбран первым, затем 34, затем 2..6
	if !reflect.DeepEqual(ticket.Numbers, want) {
		t.Fatalf("numbers = %v, хотим %v", ticket.Numbers, want)
	}
	// Проверяем суммарные веса на каждом шаге: 135, 34, 33, 32, 31, 30, 29, затем бонус 54.
	wantCalls := []int{135, 34, 33, 32, 31, 30, 29, 54}
	if !reflect.DeepEqual(rng.calls, wantCalls) {
		t.Fatalf("calls = %v, хотим %v", rng.calls, wantCalls)
	}
}

func TestGenerateTicketNoDuplicates(t *testing.T) {
	rng := &fakeRng{vals: []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}}
	ticket := GenerateTicket(make([]int, 35), make([]int, 54), rng)
	seen := map[int]bool{}
	for _, n := range ticket.Numbers {
		if seen[n] {
			t.Fatalf("повтор числа %d в %v", n, ticket.Numbers)
		}
		seen[n] = true
		if n < 1 || n > 35 {
			t.Fatalf("число %d вне 1–35", n)
		}
	}
	if ticket.Bonus < 1 || ticket.Bonus > 54 {
		t.Fatalf("бонус %d вне 1–54", ticket.Bonus)
	}
}

func TestGenerateBatchDistinct(t *testing.T) {
	// Циклический rng: каждый вызов возвращает счётчик по модулю n.
	rng := &counterRng{}
	tickets := GenerateBatch(make([]int, 35), make([]int, 54), 10, rng)
	if len(tickets) != 10 {
		t.Fatalf("len = %d", len(tickets))
	}
	unique := map[string]bool{}
	for _, tk := range tickets {
		unique[ticketKey(tk)] = true
	}
	if len(unique) != 10 {
		t.Fatalf("уникальных билетов %d из 10", len(unique))
	}
}

type counterRng struct{ c int }

func (f *counterRng) Intn(n int) int {
	v := f.c % n
	f.c++
	return v
}

func TestGenerateBatchAcceptsDupAfterCap(t *testing.T) {
	// rng всегда 0 → все билеты одинаковы; после 20 попыток принимаем дубликат.
	rng := &fakeRng{}
	tickets := GenerateBatch(make([]int, 35), make([]int, 54), 3, rng)
	if len(tickets) != 3 {
		t.Fatalf("len = %d", len(tickets))
	}
	for _, tk := range tickets {
		if !reflect.DeepEqual(tk.Numbers, []int{1, 2, 3, 4, 5, 6, 7}) || tk.Bonus != 1 {
			t.Fatalf("неожиданный билет %+v", tk)
		}
	}
	if len(rng.calls) != (1+2*dupAttempts)*8 {
		t.Fatalf("вызовов rng = %d, хотим %d", len(rng.calls), (1+2*dupAttempts)*8)
	}
}

func TestFrequenciesIgnoresOutOfRange(t *testing.T) {
	// Довложение из ревью Task 4: ветка фильтрации вне диапазона не была покрыта.
	main, bonus := Frequencies([]DrawFreq{{Numbers: []int{0, 36, 1}, Bonus: 0}})
	totalMain := 0
	for _, c := range main {
		totalMain += c
	}
	if totalMain != 1 || main[0] != 1 {
		t.Fatalf("ожидали только число 1 в счётчиках, total=%d main[0]=%d", totalMain, main[0])
	}
	totalBonus := 0
	for _, c := range bonus {
		totalBonus += c
	}
	if totalBonus != 0 {
		t.Fatalf("бонус 0 не должен считаться, total=%d", totalBonus)
	}
}

func TestGenerateBatchSingle(t *testing.T) {
	tickets := GenerateBatch(make([]int, 35), make([]int, 54), 1, &fakeRng{})
	if len(tickets) != 1 {
		t.Fatalf("len = %d", len(tickets))
	}
}
