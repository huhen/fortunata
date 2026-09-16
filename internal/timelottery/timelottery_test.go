// Тесты парсера архива timelottery.ru; фикстуры — вырезки реальной страницы.
package timelottery

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseHappyPath(t *testing.T) {
	draws, issues, err := Parse(strings.NewReader(readFixture(t, "archive.html")))
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %+v, хотели пусто", issues)
	}
	want := []Draw{
		{No: 64, Numbers: []int{5, 10, 19, 21, 24, 28, 29}, Bonus: 18},
		{No: 63, Numbers: []int{11, 12, 16, 28, 29, 31, 35}, Bonus: 14},
	}
	if len(draws) != len(want) {
		t.Fatalf("draws = %+v, хотели %d розыгрышей", draws, len(want))
	}
	for i := range want {
		if draws[i].No != want[i].No || !reflect.DeepEqual(draws[i].Numbers, want[i].Numbers) ||
			draws[i].Bonus != want[i].Bonus {
			t.Fatalf("draws[%d] = %+v, хотели %+v", i, draws[i], want[i])
		}
	}
}

func TestParseIssues(t *testing.T) {
	draws, issues, err := Parse(strings.NewReader(readFixture(t, "broken.html")))
	if err != nil {
		// Все строки похожи на данные — это не структурный отказ.
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(draws) != 0 {
		t.Fatalf("draws = %+v, хотели пусто", draws)
	}
	if len(issues) != 4 {
		t.Fatalf("issues = %+v, хотели 4", issues)
	}
	for _, tc := range []struct {
		no    int64
		fragm string
	}{
		{62, "36 вне диапазона"},
		{61, "повторяется"},
		{60, "бонусное число 55 вне диапазона"},
		{59, "0 вне диапазона"},
	} {
		found := false
		for _, is := range issues {
			if is.DrawNo == tc.no && strings.Contains(is.Reason, tc.fragm) {
				found = true
			}
		}
		if !found {
			t.Errorf("нет issue для №%d с «%s»: %+v", tc.no, tc.fragm, issues)
		}
	}
}

func TestParseStructuralError(t *testing.T) {
	draws, issues, err := Parse(strings.NewReader("<html><body><p>Пусто</p></body></html>"))
	if err == nil {
		t.Fatalf("хотели структурную ошибку, получили draws=%+v issues=%+v", draws, issues)
	}
	if !strings.Contains(err.Error(), "не найдены результаты") {
		t.Fatalf("неожиданный текст ошибки: %v", err)
	}
}
