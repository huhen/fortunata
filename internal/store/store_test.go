package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	st, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestCreateAndList(t *testing.T) {
	st := openTest(t)
	err := st.Create(Draw{DrawNo: 5, Numbers: []int{7, 3, 34, 19, 26, 2, 14}, Bonus: 48})
	if err != nil {
		t.Fatal(err)
	}
	draws, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(draws) != 1 {
		t.Fatalf("ожидали 1 розыгрыш, got %d", len(draws))
	}
	d := draws[0]
	if d.DrawNo != 5 || d.Bonus != 48 {
		t.Fatalf("не те поля: %+v", d)
	}
	want := []int{2, 3, 7, 14, 19, 26, 34} // записываются отсортированными
	for i := range want {
		if d.Numbers[i] != want[i] {
			t.Fatalf("numbers = %v, хотим %v", d.Numbers, want)
		}
	}
}

func TestListSortedDesc(t *testing.T) {
	st := openTest(t)
	for _, no := range []int64{1, 10, 3} {
		if err := st.Create(Draw{DrawNo: no, Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 8}); err != nil {
			t.Fatal(err)
		}
	}
	draws, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if draws[0].DrawNo != 10 || draws[1].DrawNo != 3 || draws[2].DrawNo != 1 {
		t.Fatalf("порядок нарушен: %v", draws)
	}
}

func TestCreateDuplicate(t *testing.T) {
	st := openTest(t)
	d := Draw{DrawNo: 5, Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 8}
	if err := st.Create(d); err != nil {
		t.Fatal(err)
	}
	err := st.Create(d)
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("ожидали ErrDuplicate, got %v", err)
	}
}

func TestCheckConstraints(t *testing.T) {
	st := openTest(t)
	if err := st.Create(Draw{DrawNo: 1, Numbers: []int{1, 2, 3, 4, 5, 6, 36}, Bonus: 8}); err == nil {
		t.Fatal("ожидали ошибку CHECK: 36 вне 1–35")
	}
	if err := st.Create(Draw{DrawNo: 1, Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 55}); err == nil {
		t.Fatal("ожидали ошибку CHECK: бонус 55 вне 1–54")
	}
}

func TestCreateNormalizeError(t *testing.T) {
	st := openTest(t)
	err := st.Create(Draw{DrawNo: 1, Numbers: []int{1, 2, 3, 4, 5, 6}, Bonus: 8})
	if err == nil {
		t.Fatal("ожидали ошибку normalize: чисел 6, а нужно 7")
	}
	if err.Error() != "нужно ровно 7 чисел" {
		t.Fatalf("не та ошибка: %v", err)
	}
}

func TestListEmpty(t *testing.T) {
	st := openTest(t)
	draws, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if draws == nil {
		t.Fatal("List на пустой базе вернул nil, хотим непустой пустой срез")
	}
	if len(draws) != 0 {
		t.Fatalf("ожидали 0 розыгрышей, got %d", len(draws))
	}
}

func TestCreateDrawNoZero(t *testing.T) {
	st := openTest(t)
	err := st.Create(Draw{DrawNo: 0, Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 8})
	if err == nil {
		t.Fatal("ожидали ошибку CHECK: draw_no=0")
	}
}

func TestUpdate(t *testing.T) {
	st := openTest(t)
	if err := st.Create(Draw{DrawNo: 5, Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 8}); err != nil {
		t.Fatal(err)
	}
	if err := st.Update(5, []int{9, 8, 7, 6, 5, 4, 3}, 54); err != nil {
		t.Fatal(err)
	}
	draws, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	d := draws[0]
	want := []int{3, 4, 5, 6, 7, 8, 9}
	for i := range want {
		if d.Numbers[i] != want[i] {
			t.Fatalf("numbers = %v, хотим %v", d.Numbers, want)
		}
	}
	if d.Bonus != 54 {
		t.Fatalf("bonus = %d", d.Bonus)
	}
	if err := st.Update(999, []int{1, 2, 3, 4, 5, 6, 7}, 8); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ожидали ErrNotFound, got %v", err)
	}
}

func TestDelete(t *testing.T) {
	st := openTest(t)
	if err := st.Create(Draw{DrawNo: 5, Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 8}); err != nil {
		t.Fatal(err)
	}
	if err := st.Delete(5); err != nil {
		t.Fatal(err)
	}
	draws, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(draws) != 0 {
		t.Fatalf("после удаления осталось %d", len(draws))
	}
	if err := st.Delete(5); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ожидали ErrNotFound, got %v", err)
	}
}

func TestCreateDuplicateNumbersRejected(t *testing.T) {
	st := openTest(t)
	// После сортировки [1,1,2,3,4,5,6] остаётся с повтором —
	// строго возрастающий CHECK на уровне БД отклоняет.
	err := st.Create(Draw{DrawNo: 1, Numbers: []int{1, 1, 2, 3, 4, 5, 6}, Bonus: 8})
	if err == nil {
		t.Fatal("ожидали отказ CHECK: числа семёрки не различны")
	}
}

func TestOpenFileRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Create(Draw{DrawNo: 7, Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 8}); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	draws, err := st2.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(draws) != 1 || draws[0].DrawNo != 7 {
		t.Fatalf("после переоткрытия: %+v", draws)
	}
}

func TestMainFrequency(t *testing.T) {
	st := openTest(t)
	if err := st.Create(Draw{DrawNo: 1, Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 8}); err != nil {
		t.Fatal(err)
	}
	if err := st.Create(Draw{DrawNo: 2, Numbers: []int{1, 2, 3, 8, 9, 10, 11}, Bonus: 8}); err != nil {
		t.Fatal(err)
	}
	got, err := st.MainFrequency()
	if err != nil {
		t.Fatal(err)
	}
	// 1,2,3 — по два раза; остальные по одному; при равенстве — номер ASC.
	// Числа 12–35 не выпадали и в результат попасть не должны.
	// Бонус 8 совпадает с основным шаром 8 намеренно: просочки бонусной
	// колонки в UNION подняли бы счётчик 8 до 3 и уронили бы тест.
	want := []Freq{
		{N: 1, Count: 2}, {N: 2, Count: 2}, {N: 3, Count: 2},
		{N: 4, Count: 1}, {N: 5, Count: 1}, {N: 6, Count: 1}, {N: 7, Count: 1},
		{N: 8, Count: 1}, {N: 9, Count: 1}, {N: 10, Count: 1}, {N: 11, Count: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, хотим %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("[%d] = %+v, хотим %+v (порядок: count DESC, затем номер ASC)", i, got[i], want[i])
		}
	}
}

func TestFrequencyEmptyBase(t *testing.T) {
	st := openTest(t)
	main, err := st.MainFrequency()
	if err != nil {
		t.Fatal(err)
	}
	if main == nil || len(main) != 0 {
		t.Fatalf("пустая база: main = %v, хотим непустой пустой срез", main)
	}
}
