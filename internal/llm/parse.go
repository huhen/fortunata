// Парсинг ответа LLM: извлечение JSON с комбинациями и валидация билетов.

package llm

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"fortunata/internal/generate"
)

// thinkRe срезает парные <think>-блоки (qwen3 рассуждает перед ответом),
// unclosedThinkRe — незакрытый <think> до конца ответа.
var (
	thinkRe         = regexp.MustCompile(`(?s)<think>.*?</think>`)
	unclosedThinkRe = regexp.MustCompile(`(?s)<think>.*`)
)

// combinationsPayload — ожидаемая структура ответа модели.
type combinationsPayload struct {
	Combinations []struct {
		Numbers []int `json:"numbers"`
		Bonus   int   `json:"bonus"`
	} `json:"combinations"`
}

// ExtractTickets вытаскивает из ответа модели валидные билеты. Ответ может
// содержать <think>-рассуждения, ```-заборы и текст вокруг JSON. Невалидные
// комбинации отбрасываются; у валидных числа сортируются по возрастанию.
func ExtractTickets(content string) []generate.Ticket {
	cleaned := unclosedThinkRe.ReplaceAllString(thinkRe.ReplaceAllString(content, ""), "")
	start := strings.Index(cleaned, "{")
	if start < 0 {
		return nil
	}
	var payload combinationsPayload
	// Decode читает первый JSON-объект и игнорирует мусор после него.
	if err := json.NewDecoder(strings.NewReader(cleaned[start:])).Decode(&payload); err != nil {
		return nil
	}
	tickets := make([]generate.Ticket, 0, len(payload.Combinations))
	for _, c := range payload.Combinations {
		if t, ok := validTicket(c.Numbers, c.Bonus); ok {
			tickets = append(tickets, t)
		}
	}
	return tickets
}

// validTicket проверяет комбинацию: ровно 7 уникальных чисел 1–35, бонус 1–54.
func validTicket(numbers []int, bonus int) (generate.Ticket, bool) {
	if len(numbers) != 7 || bonus < 1 || bonus > 54 {
		return generate.Ticket{}, false
	}
	seen := make(map[int]struct{}, 7)
	for _, n := range numbers {
		if n < 1 || n > 35 {
			return generate.Ticket{}, false
		}
		if _, dup := seen[n]; dup {
			return generate.Ticket{}, false
		}
		seen[n] = struct{}{}
	}
	sorted := append([]int(nil), numbers...)
	sort.Ints(sorted)
	return generate.Ticket{Numbers: sorted, Bonus: bonus}, true
}
