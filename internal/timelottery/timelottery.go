// Пакет timelottery: разбор страницы архива розыгрышей timelottery.ru
// (https://timelottery.ru/arhiv/rezultaty-vseh-rozygryshej-fortunata/).
//
// Разбор не привязан к классам, стилям, порядку колонок и номеру таблицы:
// строкой данных считается <tr>, у которого первая ячейка — целое число ≥ 1,
// а среди остальных ровно одна ячейка с восемью числами (семёрка + бонус);
// если таких ячеек несколько — строка уходит в Issue (неоднозначность).
// У остальных ячеек такой плотности цифр не бывает: дата «14 сент» → 1 число,
// «10 млн» → 1, «93,3 млн» → 2, «архив (#64)» → 1.
package timelottery

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// Draw — розыгрыш, извлечённый из архива.
type Draw struct {
	No      int64 // номер розыгрыша
	Numbers []int // 7 основных чисел 1–35, по возрастанию
	Bonus   int   // бонусное число 1–54
}

// Issue — похожая на данные строка с невалидной комбинацией.
type Issue struct {
	DrawNo int64  // номер розыгрыша из первой ячейки
	Reason string // человекочитаемая причина
}

// Parse разбирает HTML архива: возвращает розыгрыши и проблемы. Ошибка —
// структурная: похожих на данные строк нет вовсе (сайт изменился).
func Parse(r io.Reader) ([]Draw, []Issue, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, nil, fmt.Errorf("разбор html: %w", err)
	}
	var (
		draws  []Draw
		issues []Issue
	)
	for _, cells := range tableRows(doc) {
		if len(cells) < 2 {
			continue
		}
		no, err := strconv.ParseInt(strings.TrimSpace(cells[0]), 10, 64)
		if err != nil || no < 1 {
			continue // шапка, сноска или строка другой таблицы
		}
		nums, count := numbersCell(cells[1:])
		if count == 0 {
			continue // не похоже на строку данных
		}
		if count > 1 {
			issues = append(issues, Issue{
				DrawNo: no,
				Reason: fmt.Sprintf("неоднозначно: найдено %d ячеек с восемью числами", count),
			})
			continue
		}
		main := nums[:7]
		if msg := validate(main, nums[7]); msg != "" {
			issues = append(issues, Issue{DrawNo: no, Reason: msg})
			continue
		}
		sort.Ints(main)
		draws = append(draws, Draw{No: no, Numbers: main, Bonus: nums[7]})
	}
	if len(draws) == 0 && len(issues) == 0 {
		return nil, nil, errors.New("на странице не найдены результаты розыгрышей")
	}
	return draws, issues, nil
}

// numbersCell возвращает первую из ячеек с ровно восемью числами
// (семёрка + бонус); count — сколько таких ячеек всего.
func numbersCell(cells []string) (nums []int, count int) {
	for _, text := range cells {
		ns := extractNumbers(text)
		if len(ns) == 8 {
			if count == 0 {
				nums = ns
			}
			count++
		}
	}
	return nums, count
}

// validate — первые 7 чисел в 1–35 без повторов, бонус в 1–54;
// "" если комбинация валидна, иначе причина для Issue.
func validate(main []int, bonus int) string {
	seen := make(map[int]struct{}, 7)
	for _, n := range main {
		if n < 1 || n > 35 {
			return fmt.Sprintf("число %d вне диапазона 1–35", n)
		}
		if _, dup := seen[n]; dup {
			return fmt.Sprintf("число %d повторяется", n)
		}
		seen[n] = struct{}{}
	}
	if bonus < 1 || bonus > 54 {
		return fmt.Sprintf("бонусное число %d вне диапазона 1–54", bonus)
	}
	return ""
}

// tableRows возвращает тексты ячеек каждой <tr> в порядке следования.
func tableRows(n *html.Node) [][]string {
	var rows [][]string
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tr" {
			rows = append(rows, rowCells(n))
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(n)
	return rows
}

// rowCells — тексты <td>/<th> строки в порядке следования.
func rowCells(tr *html.Node) []string {
	var out []string
	for c := tr.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
			out = append(out, nodeText(c))
		}
	}
	return out
}

// nodeText — конкатенация текстовых узлов поддерева (плоский текст ячейки).
func nodeText(n *html.Node) string {
	var b strings.Builder
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(n)
	return b.String()
}

// extractNumbers — все последовательности цифр в тексте как числа
// (разделителем считается любая подстрока без цифр: запятые, «и», пробелы).
func extractNumbers(s string) []int {
	var out []int
	start := -1 // байтовый индекс начала текущей группы цифр
	for i, r := range s {
		if r >= '0' && r <= '9' {
			if start < 0 {
				start = i
			}
		} else if start >= 0 {
			n, _ := strconv.Atoi(s[start:i])
			out = append(out, n)
			start = -1
		}
	}
	if start >= 0 {
		n, _ := strconv.Atoi(s[start:])
		out = append(out, n)
	}
	return out
}
