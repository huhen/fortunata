// Пакет auth: проверка пароля и подписанные cookie-сессии без серверного хранилища.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

const (
	CookieName = "session"
	SessionTTL = 7 * 24 * time.Hour
)

type Manager struct {
	passwordHash [32]byte
	secret       []byte
	now          func() time.Time // заменяется в тестах
}

// New создаёт Manager. password и secret должны быть непустыми;
// валидация на стороне конфигурации приложения.
func New(password, secret string) *Manager {
	return &Manager{
		passwordHash: sha256.Sum256([]byte(password)),
		secret:       []byte(secret),
		now:          time.Now,
	}
}

// CheckPassword сравнивает пароль в constant-time (через sha256, чтобы выровнять длину).
func (m *Manager) CheckPassword(password string) bool {
	h := sha256.Sum256([]byte(password))
	return subtle.ConstantTimeCompare(h[:], m.passwordHash[:]) == 1
}

// NewToken возвращает "<expiry-unix>.<hmac-sha256(expiry)>".
func (m *Manager) NewToken() string {
	msg := strconv.FormatInt(m.now().Add(SessionTTL).Unix(), 10)
	return msg + "." + hex.EncodeToString(m.sign(msg))
}

// Verify проверяет подпись и срок годности токена.
func (m *Manager) Verify(token string) bool {
	msg, sigHex, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}
	exp, err := strconv.ParseInt(msg, 10, 64)
	if err != nil || m.now().Unix() > exp {
		return false
	}
	return subtle.ConstantTimeCompare(sig, m.sign(msg)) == 1
}

func (m *Manager) sign(msg string) []byte {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(msg))
	return mac.Sum(nil)
}
