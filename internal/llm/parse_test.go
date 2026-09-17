// Тесты парсинга ответа LLM: чистый JSON, <think>-блоки qwen3, заборы,
// мусор вокруг, невалидные комбинации.
package llm

import (
	"reflect"
	"testing"
)

func TestExtractTicketsCleanJSON(t *testing.T) {
	content := `{"combinations":[{"numbers":[3,7,12,19,25,31,34],"bonus":8},{"numbers":[1,2,3,4,5,6,7],"bonus":54}]}`
	got := ExtractTickets(content)
	if len(got) != 2 {
		t.Fatalf("билетов = %d, хотим 2", len(got))
	}
	if !reflect.DeepEqual(got[0].Numbers, []int{3, 7, 12, 19, 25, 31, 34}) || got[0].Bonus != 8 {
		t.Fatalf("билет 0 = %+v", got[0])
	}
}

func TestExtractTicketsThinkAndFences(t *testing.T) {
	content := "<think>Мне нужно выбрать числа… {\"fake\": true}</think>\n```json\n" +
		`{"combinations":[{"numbers":[34,31,25,19,12,7,3],"bonus":8}]}` + "\n```\nВот ваши числа!"
	got := ExtractTickets(content)
	if len(got) != 1 {
		t.Fatalf("билетов = %d, хотим 1", len(got))
	}
	// Числа пришли в обратном порядке — сортировка применена.
	if !reflect.DeepEqual(got[0].Numbers, []int{3, 7, 12, 19, 25, 31, 34}) {
		t.Fatalf("numbers = %v", got[0].Numbers)
	}
}

func TestExtractTicketsGarbageAroundJSON(t *testing.T) {
	content := `Sure! Here are your numbers: {"combinations":[{"numbers":[1,2,3,4,5,6,7],"bonus":8}]} Hope this helps!`
	got := ExtractTickets(content)
	if len(got) != 1 {
		t.Fatalf("билетов = %d, хотим 1", len(got))
	}
}

func TestExtractTicketsDropsInvalid(t *testing.T) {
	content := `{"combinations":[
		{"numbers":[1,2,3],"bonus":8},
		{"numbers":[1,2,3,4,5,6,36],"bonus":8},
		{"numbers":[1,2,3,4,5,6,7],"bonus":55},
		{"numbers":[1,1,3,4,5,6,7],"bonus":8},
		{"numbers":[1,2,3,4,5,6,7],"bonus":8}
	]}`
	got := ExtractTickets(content)
	if len(got) != 1 || got[0].Bonus != 8 {
		t.Fatalf("билеты = %+v, хотим 1 валидный", got)
	}
}

func TestExtractTicketsNoJSON(t *testing.T) {
	if got := ExtractTickets("Извините, не могу помочь."); len(got) != 0 {
		t.Fatalf("билеты = %+v, хотим пусто", got)
	}
	if got := ExtractTickets("<think>думаю"); len(got) != 0 {
		t.Fatalf("незакрытый think: %+v, хотим пусто", got)
	}
}
