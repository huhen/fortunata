// Тесты промта: правила, статистика, exclude-список.
package llm

import (
	"strings"
	"testing"

	"fortunata/internal/generate"
)

func TestComposePromptContainsRulesAndStats(t *testing.T) {
	main := make([]int, 35)
	main[0] = 5 // число 1 выпадало 5 раз
	bonus := make([]int, 54)
	bonus[7] = 2 // бонус 8 выпадал 2 раза
	p := ComposePrompt(10, main, bonus, 123, nil)
	for _, want := range []string{
		"Past draws analyzed: 123",
		"Propose 10 distinct combinations",
		"1:5", "8:2",
		"7 distinct main numbers", "ascending", "bonus number from 1 to 54",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("в промте нет %q\nпромт:\n%s", want, p)
		}
	}
}

func TestComposePromptExcludeList(t *testing.T) {
	exclude := []generate.Ticket{{Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 8}}
	p := ComposePrompt(3, make([]int, 35), make([]int, 54), 1, exclude)
	for _, want := range []string{"do not repeat", "[1 2 3 4 5 6 7] bonus 8", "Propose 3 distinct"} {
		if !strings.Contains(p, want) {
			t.Fatalf("в промте нет %q\nпромт:\n%s", want, p)
		}
	}
}
