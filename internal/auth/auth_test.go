package auth

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func newTest() *Manager {
	m := New("pass123", "test-secret")
	return m
}

func TestCheckPassword(t *testing.T) {
	m := newTest()
	if !m.CheckPassword("pass123") {
		t.Fatal("верный пароль не принят")
	}
	if m.CheckPassword("wrong") || m.CheckPassword("") {
		t.Fatal("неверный пароль принят")
	}
}

func TestTokenRoundTrip(t *testing.T) {
	m := newTest()
	tok := m.NewToken()
	if !m.Verify(tok) {
		t.Fatal("свежий токен не прошёл проверку")
	}
}

func TestTokenGarbage(t *testing.T) {
	m := newTest()
	for _, tok := range []string{"", "abc", "1.zzz", "abc.def", "123."} {
		if m.Verify(tok) {
			t.Fatalf("мусор принят: %q", tok)
		}
	}
}

func TestTokenTampered(t *testing.T) {
	m := newTest()
	tok := m.NewToken()
	msg, sigHex, _ := strings.Cut(tok, ".")
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		t.Fatal(err)
	}
	sig[0] ^= 0xff
	bad := msg + "." + hex.EncodeToString(sig)
	if m.Verify(bad) {
		t.Fatal("подделанная подпись принята")
	}
	// Подмена срока годности при сохранённой подписи.
	if m.Verify("9999999999." + sigHex) {
		t.Fatal("токен с подменённым сроком принят")
	}
}

func TestTokenExpiry(t *testing.T) {
	m := newTest()
	now := time.Unix(1_700_000_000, 0)
	m.now = func() time.Time { return now }
	tok := m.NewToken()
	if !m.Verify(tok) {
		t.Fatal("токен должен быть жив до истечения")
	}
	m.now = func() time.Time { return now.Add(SessionTTL) }
	if !m.Verify(tok) {
		t.Fatal("токен должен быть жив ровно в секунду истечения")
	}
	m.now = func() time.Time { return now.Add(SessionTTL + time.Second) }
	if m.Verify(tok) {
		t.Fatal("токен должен умереть через секунду после истечения")
	}
}

func TestTokenExpiredAtPastExpiry(t *testing.T) {
	m := newTest()
	now := time.Unix(1_700_000_000, 0)
	m.now = func() time.Time { return now }
	if m.Verify("1." + strings.Repeat("a", 64)) {
		t.Fatal("токен с expiry=1 (1970 год) принят")
	}
}

func TestSecretSeparatesTokens(t *testing.T) {
	tok := New("p", "secret-a").NewToken()
	if New("p", "secret-b").Verify(tok) {
		t.Fatal("токен, подписанный другим секретом, принят")
	}
}
