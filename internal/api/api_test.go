package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"fortunata/internal/store"
)

// newTestServer поднимает api на httptest с паролем "pass123".
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ts := httptest.NewServer(New(st, "pass123", "test-secret", false, ""))
	t.Cleanup(ts.Close)
	return ts
}

// clientWithJar — клиент, хранящий cookie между запросами.
func clientWithJar() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

func post(t *testing.T, c *http.Client, url string, body any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func get(t *testing.T, c *http.Client, url string) *http.Response {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestLoginWrongPassword(t *testing.T) {
	ts := newTestServer(t)
	c := clientWithJar()
	start := time.Now()
	resp := post(t, c, ts.URL+"/api/login", map[string]string{"password": "nope"})
	elapsed := time.Since(start)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if elapsed < 900*time.Millisecond {
		t.Fatalf("нет паузы против перебора: %v", elapsed)
	}
}

func TestLoginSetsCookie(t *testing.T) {
	ts := newTestServer(t)
	c := clientWithJar()
	resp := post(t, c, ts.URL+"/api/login", map[string]string{"password": "pass123"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	// HttpOnly проверяем по заголовку ответа: cookiejar в Jar.Cookies()
	// возвращает только Name/Value/Quoted, атрибуты недостижимы.
	var session *http.Cookie
	for _, ck := range resp.Cookies() {
		if ck.Name == "session" {
			session = ck
			break
		}
	}
	if session == nil {
		t.Fatal("cookie session не установлена")
	}
	if !session.HttpOnly {
		t.Fatal("cookie должна быть HttpOnly")
	}
	if session.MaxAge != 7*24*3600 {
		t.Fatalf("Max-Age = %d, хотим SessionTTL", session.MaxAge)
	}
	if len(c.Jar.Cookies(mustURL(t, ts.URL))) == 0 {
		t.Fatal("jar не сохранил cookie")
	}
}

func TestMeReflectsSession(t *testing.T) {
	ts := newTestServer(t)
	anon := clientWithJar()
	resp := get(t, anon, ts.URL+"/api/me")
	var body struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if body.Authenticated {
		t.Fatal("аноним не должен быть аутентифицирован")
	}

	authed := clientWithJar()
	resp2 := post(t, authed, ts.URL+"/api/login", map[string]string{"password": "pass123"})
	resp2.Body.Close()
	resp3 := get(t, authed, ts.URL+"/api/me")
	var body2 struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := json.NewDecoder(resp3.Body).Decode(&body2); err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()
	if !body2.Authenticated {
		t.Fatal("после входа /api/me должен вернуть true")
	}
}

func TestLogoutClearsSession(t *testing.T) {
	ts := newTestServer(t)
	c := clientWithJar()
	resp := post(t, c, ts.URL+"/api/login", map[string]string{"password": "pass123"})
	resp.Body.Close()
	resp = post(t, c, ts.URL+"/api/logout", map[string]any{})
	resp.Body.Close()
	resp = get(t, c, ts.URL+"/api/me")
	var body struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if body.Authenticated {
		t.Fatal("после logout сессия должна быть сброшена")
	}
}

func TestMutationsRequireJSONContentType(t *testing.T) {
	ts := newTestServer(t)
	resp, err := http.Post(ts.URL+"/api/login", "text/plain", bytes.NewBufferString(`{"password":"pass123"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, хотим 415", resp.StatusCode)
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// doReq — запрос с произвольным методом и JSON-телом (тело может быть nil).
func doReq(t *testing.T, c *http.Client, method, url string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// loginClient возвращает клиент с активной сессией.
func loginClient(t *testing.T, ts *httptest.Server) *http.Client {
	t.Helper()
	c := clientWithJar()
	resp := post(t, c, ts.URL+"/api/login", map[string]string{"password": "pass123"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login status = %d", resp.StatusCode)
	}
	return c
}

func validCombo() map[string]any {
	return map[string]any{"drawNo": 12, "numbers": []int{7, 3, 34, 19, 26, 2, 14}, "bonus": 48}
}

func TestCreateDraw(t *testing.T) {
	ts := newTestServer(t)
	c := loginClient(t, ts)
	resp := post(t, c, ts.URL+"/api/draws", validCombo())
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var created struct {
		DrawNo  int64 `json:"drawNo"`
		Numbers []int `json:"numbers"`
		Bonus   int   `json:"bonus"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.DrawNo != 12 || created.Bonus != 48 {
		t.Fatalf("created = %+v", created)
	}
	want := []int{2, 3, 7, 14, 19, 26, 34} // ответ с отсортированной семёркой
	for i := range want {
		if created.Numbers[i] != want[i] {
			t.Fatalf("numbers = %v", created.Numbers)
		}
	}
	// Публичное чтение видит созданный розыгрыш.
	resp2 := get(t, clientWithJar(), ts.URL+"/api/draws")
	var list struct {
		Draws []struct {
			DrawNo int64 `json:"drawNo"`
		} `json:"draws"`
	}
	_ = json.NewDecoder(resp2.Body).Decode(&list)
	resp2.Body.Close()
	if len(list.Draws) != 1 || list.Draws[0].DrawNo != 12 {
		t.Fatalf("list = %+v", list.Draws)
	}
}

func TestCreateDrawRequiresSession(t *testing.T) {
	ts := newTestServer(t)
	resp := post(t, clientWithJar(), ts.URL+"/api/draws", validCombo())
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestCreateDrawDuplicate(t *testing.T) {
	ts := newTestServer(t)
	c := loginClient(t, ts)
	resp := post(t, c, ts.URL+"/api/draws", validCombo())
	resp.Body.Close()
	resp = post(t, c, ts.URL+"/api/draws", validCombo())
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.Error == "" {
		t.Fatal("ожидали сообщение об ошибке")
	}
}

func TestCreateDrawValidation(t *testing.T) {
	ts := newTestServer(t)
	c := loginClient(t, ts)
	cases := []struct {
		name string
		body map[string]any
	}{
		{"мало чисел", map[string]any{"drawNo": 1, "numbers": []int{1, 2, 3}, "bonus": 8}},
		{"вне диапазона", map[string]any{"drawNo": 1, "numbers": []int{1, 2, 3, 4, 5, 6, 36}, "bonus": 8}},
		{"повтор числа", map[string]any{"drawNo": 1, "numbers": []int{1, 1, 3, 4, 5, 6, 7}, "bonus": 8}},
		{"бонус вне диапазона", map[string]any{"drawNo": 1, "numbers": []int{1, 2, 3, 4, 5, 6, 7}, "bonus": 55}},
		{"нулевой номер", map[string]any{"drawNo": 0, "numbers": []int{1, 2, 3, 4, 5, 6, 7}, "bonus": 8}},
	}
	for _, tc := range cases {
		resp := post(t, c, ts.URL+"/api/draws", tc.body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: status = %d", tc.name, resp.StatusCode)
		}
	}
}

func TestUpdateDraw(t *testing.T) {
	ts := newTestServer(t)
	c := loginClient(t, ts)
	resp := post(t, c, ts.URL+"/api/draws", validCombo())
	resp.Body.Close()
	resp = doReq(t, c, "PUT", ts.URL+"/api/draws/12",
		map[string]any{"numbers": []int{9, 8, 7, 6, 5, 4, 3}, "bonus": 54})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	resp3 := get(t, clientWithJar(), ts.URL+"/api/draws")
	defer resp3.Body.Close()
	var list struct {
		Draws []struct {
			DrawNo  int64 `json:"drawNo"`
			Numbers []int `json:"numbers"`
			Bonus   int   `json:"bonus"`
		} `json:"draws"`
	}
	if err := json.NewDecoder(resp3.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Draws) != 1 || list.Draws[0].DrawNo != 12 || list.Draws[0].Bonus != 54 {
		t.Fatalf("после update: %+v", list.Draws)
	}
	resp = doReq(t, c, "PUT", ts.URL+"/api/draws/999",
		map[string]any{"numbers": []int{1, 2, 3, 4, 5, 6, 7}, "bonus": 8})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing: status = %d", resp.StatusCode)
	}
}

func TestDeleteDraw(t *testing.T) {
	ts := newTestServer(t)
	c := loginClient(t, ts)
	resp := post(t, c, ts.URL+"/api/draws", validCombo())
	resp.Body.Close()
	resp = doReq(t, c, "DELETE", ts.URL+"/api/draws/12", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	resp = doReq(t, c, "DELETE", ts.URL+"/api/draws/12", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("повторное удаление: status = %d", resp.StatusCode)
	}
}

func TestUpdateDrawBadPathValue(t *testing.T) {
	ts := newTestServer(t)
	c := loginClient(t, ts)
	resp := doReq(t, c, "PUT", ts.URL+"/api/draws/abc",
		map[string]any{"numbers": []int{1, 2, 3, 4, 5, 6, 7}, "bonus": 8})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestCreateDrawBadJSON(t *testing.T) {
	ts := newTestServer(t)
	c := loginClient(t, ts)
	req, err := http.NewRequest("POST", ts.URL+"/api/draws", bytes.NewBufferString("{нет json"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestGenerate(t *testing.T) {
	ts := newTestServer(t)
	resp := post(t, clientWithJar(), ts.URL+"/api/generate", map[string]any{"count": 3})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Tickets []struct {
			Numbers []int `json:"numbers"`
			Bonus   int   `json:"bonus"`
		} `json:"tickets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Tickets) != 3 {
		t.Fatalf("билетов: %d", len(body.Tickets))
	}
	for _, tk := range body.Tickets {
		if len(tk.Numbers) != 7 {
			t.Fatalf("чисел в билете: %d", len(tk.Numbers))
		}
		prev := 0
		for _, n := range tk.Numbers {
			if n <= prev || n < 1 || n > 35 {
				t.Fatalf("семёрка не отсортирована/вне диапазона: %v", tk.Numbers)
			}
			prev = n
		}
		if tk.Bonus < 1 || tk.Bonus > 54 {
			t.Fatalf("бонус %d вне 1–54", tk.Bonus)
		}
	}
}

func TestGenerateValidation(t *testing.T) {
	ts := newTestServer(t)
	c := clientWithJar()
	for _, count := range []int{0, -1, 101} {
		resp := post(t, c, ts.URL+"/api/generate", map[string]any{"count": count})
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("count=%d: status = %d", count, resp.StatusCode)
		}
	}
	// Пустая база — валидный режим: генерация равномерная, но работает.
	resp := post(t, c, ts.URL+"/api/generate", map[string]any{"count": 1})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("пустая база: status = %d", resp.StatusCode)
	}
}

func TestGenerateUsesHistory(t *testing.T) {
	// 50 розыгрышей {1..7}+8: числа 1–7 получают вес 51, остальные — 1.
	// ГСЧ crypto/rand, поэтому проверка статистическая: в 10 билетах
	// самое частое число почти наверняка встретится.
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for i := 1; i <= 50; i++ {
		if err := st.Create(drawStruct(int64(i))); err != nil {
			t.Fatal(err)
		}
	}
	ts := httptest.NewServer(New(st, "pass123", "test-secret", false, ""))
	t.Cleanup(ts.Close)
	resp := post(t, clientWithJar(), ts.URL+"/api/generate", map[string]any{"count": 10})
	defer resp.Body.Close()
	var body struct {
		Tickets []struct {
			Numbers []int `json:"numbers"`
		} `json:"tickets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tk := range body.Tickets {
		for _, n := range tk.Numbers {
			if n == 1 {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("самое частое число 1 не встретилось ни в одном билете из 10")
	}
}

// drawStruct — розыгрыш с фиксированной комбинацией {1,2,3,4,5,6,7}+8.
func drawStruct(no int64) store.Draw {
	return store.Draw{DrawNo: no, Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 8}
}
