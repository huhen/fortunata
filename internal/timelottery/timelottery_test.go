// Тесты парсера архива timelottery.ru; фикстуры — вырезки реальной страницы.
package timelottery

import (
	"math"
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

// Две ячейки с восемью числами в одной строке — неоднозначность:
// issue вместо молчаливого «взять первую».
func TestParseAmbiguousRow(t *testing.T) {
	draws, issues, err := Parse(strings.NewReader(readFixture(t, "ambiguous.html")))
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(draws) != 0 {
		t.Fatalf("draws = %+v, хотели пусто", draws)
	}
	if len(issues) != 1 {
		t.Fatalf("issues = %+v, хотели 1", issues)
	}
	if issues[0].DrawNo != 64 || !strings.Contains(issues[0].Reason, "неоднозначно") {
		t.Fatalf("issue = %+v", issues[0])
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

func TestExtractNumbers(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want []int
	}{
		{"дата", "14 сент", []int{14}},
		{"приз с запятой", "93,3 млн", []int{93, 3}},
		{"юникод-тире", "1–2", []int{1, 2}},
		{"неразрывный пробел", "7\u00a011", []int{7, 11}},
		{"цифры в конце строки", "7 и 11", []int{7, 11}},
		{"комбинация целиком", "19, 28, 24, 21, 10, 29, 05 и 18", []int{19, 28, 24, 21, 10, 29, 5, 18}},
		// 20 цифр: Atoi сигнализирует о переполнении (ErrRange), ошибка
		// игнорируется и возвращается насыщенное MaxInt — задокументированное
		// поведение; в реальной строке такой артефакт отсекается validate.
		{"переполнение Atoi", "99999999999999999999", []int{math.MaxInt}},
		{"нет цифр", "архив", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractNumbers(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("extractNumbers(%q) = %v, хотели %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	ok := []int{5, 10, 19, 21, 24, 28, 29}
	for _, tc := range []struct {
		name  string
		main  []int
		bonus int
		fragm string // "" — валидна
	}{
		{"валидна", ok, 18, ""},
		{"число вне диапазона", []int{0, 10, 19, 21, 24, 28, 29}, 18, "0 вне диапазона"},
		{"число больше 35", []int{5, 10, 19, 21, 24, 28, 36}, 18, "36 вне диапазона"},
		{"повтор", []int{5, 5, 19, 21, 24, 28, 29}, 18, "повторяется"},
		{"бонус вне диапазона", ok, 55, "бонусное число 55 вне диапазона"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg := validate(tc.main, tc.bonus)
			if tc.fragm == "" {
				if msg != "" {
					t.Fatalf("validate(%v, %d) = %q, хотели \"\"", tc.main, tc.bonus, msg)
				}
				return
			}
			if !strings.Contains(msg, tc.fragm) {
				t.Fatalf("validate(%v, %d) = %q, хотели подстроку %q", tc.main, tc.bonus, msg, tc.fragm)
			}
		})
	}
}
