// Сборка промтов для LLM на английском: правила игры, статистика, формат ответа.

package llm

import (
	"fmt"
	"strconv"
	"strings"

	"fortunata/internal/generate"
)

// rulesLine — текст правил комбинации; используется в system и user промте.
const rulesLine = "7 distinct main numbers from 1 to 35 in ascending order, plus 1 bonus number from 1 to 54"

// systemPrompt — правила игры и требование к формату ответа.
const systemPrompt = "You are a lottery combination proposer for the Fortunata lottery.\nGame rules:\n- A ticket is " + rulesLine + ".\n- All tickets in your answer must be distinct from each other.\n\nRespond with ONLY a JSON object in exactly this format, no markdown fences, no explanations:\n{\"combinations\":[{\"numbers\":[3,7,12,19,25,31,34],\"bonus\":8}]}"

// ComposePrompt строит пользовательский промт: статистика выпадений,
// количество прошедших тиражей и просьба предложить count комбинаций.
// main — 35 значений, bonus — 54 (как возвращает generate.Frequencies).
// exclude — уже найденные комбинации: их повторять нельзя (добор).
func ComposePrompt(count int, main, bonus []int, totalDraws int, exclude []generate.Ticket) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Past draws analyzed: %d.\n", totalDraws)
	b.WriteString("Main number frequency (number:times drawn):\n")
	b.WriteString(frequencyList(main))
	b.WriteString("Bonus number frequency (number:times drawn):\n")
	b.WriteString(frequencyList(bonus))
	b.WriteString("Use these frequencies as a soft guide when choosing numbers, and keep your combinations varied.\n")
	if len(exclude) > 0 {
		b.WriteString("Already proposed combinations (do not repeat them):\n")
		for _, t := range exclude {
			fmt.Fprintf(&b, "- {\"numbers\":[%s],\"bonus\":%d}\n", joinInts(t.Numbers), t.Bonus)
		}
	}
	b.WriteString("Each combination: " + rulesLine + ".\n")
	fmt.Fprintf(&b, "Propose %d distinct combinations for the next draw.\n", count)
	return b.String()
}

// frequencyList форматирует частоты как "1:5, 2:0, ...".
func frequencyList(freq []int) string {
	parts := make([]string, len(freq))
	for i, c := range freq {
		parts[i] = fmt.Sprintf("%d:%d", i+1, c)
	}
	return strings.Join(parts, ", ") + "\n"
}

// joinInts склеивает числа через запятую: "1,2,3".
func joinInts(nums []int) string {
	parts := make([]string, len(nums))
	for i, n := range nums {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ",")
}
