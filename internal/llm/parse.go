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

// combination — одна строка «комбинация» из ответа модели.
type combination struct {
	Numbers []int `json:"numbers"`
	Bonus   int   `json:"bonus"`
}

// maxJSONCandidates — сколько вхождений { пробуем: посторонняя скобка до
// настоящего JSON ({curly} в тексте ответа) не должна стоить полного
// добора. Ответ домашней LLM короткий, десяток кандидатов — потолок с запасом.
const maxJSONCandidates = 10

// ExtractTickets вытаскивает из ответа модели валидные билеты. Ответ может
// содержать <think>-рассуждения, ```-заборы и текст вокруг JSON. Невалидные
// комбинации отбрасываются; у валидных числа сортируются по возрастанию.
func ExtractTickets(content string) []generate.Ticket {
	cleaned := unclosedThinkRe.ReplaceAllString(thinkRe.ReplaceAllString(content, ""), "")
	for attempt, pos := 0, 0; attempt < maxJSONCandidates; attempt++ {
		i := strings.Index(cleaned[pos:], "{")
		if i < 0 {
			break
		}
		pos += i
		if tickets := decodeTickets(cleaned[pos:]); len(tickets) > 0 {
			return tickets
		}
		pos++
	}
	return nil
}

// decodeTickets читает поле combinations JSON-объекта, начинающегося в s.
// Объект разбирается потоково (по полям и элементам массива), а не целиком:
// при обрезанном хвосте (finish_reason=length) сохраняются уже завершённые
// комбинации. Мусор после закрывающей } игнорируется; при ошибке разбора
// в любом месте возвращаются собранные до этого билеты (возможно, nil).
func decodeTickets(s string) []generate.Ticket {
	dec := json.NewDecoder(strings.NewReader(s))
	var tickets []generate.Ticket
	if d, ok := firstDelim(dec); !ok || d != '{' {
		return nil
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			break
		}
		if key, _ := keyTok.(string); key != "combinations" {
			var skip json.RawMessage // пропустить значение другого поля
			if err := dec.Decode(&skip); err != nil {
				break
			}
			continue
		}
		if d, ok := firstDelim(dec); !ok || d != '[' {
			break // combinations не массив
		}
		for dec.More() {
			var c combination
			if err := dec.Decode(&c); err != nil {
				return tickets // обрезанный элемент: держим завершённые
			}
			if t, ok := validTicket(c.Numbers, c.Bonus); ok {
				tickets = append(tickets, t)
			}
		}
	}
	return tickets
}

// firstDelim читает следующий токен и сообщает, структурный разделитель.
func firstDelim(dec *json.Decoder) (json.Delim, bool) {
	tok, err := dec.Token()
	if err != nil {
		return 0, false
	}
	d, ok := tok.(json.Delim)
	return d, ok
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
