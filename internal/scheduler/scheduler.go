// Пакет scheduler: ежедневный запуск функции в HH:MM UTC с почасовым
// повтором при ошибке. Ничего не знает о том, что запускает, — вызывающий
// передаёт fn.
package scheduler

import (
	"context"
	"log/slog"
	"time"
)

// Scheduler вызывает fn ежедневно в hour:min UTC. Поле now и wait — точка
// инъекции для детерминированных тестов (см. scheduler_test.go).
type Scheduler struct {
	hour, min  int
	fn         func(context.Context) error
	now        func() time.Time
	wait       func(context.Context, time.Duration) error
	retryEvery time.Duration
}

// New создаёт планировщик с реальными часами и часовым интервалом повтора.
// hour ∈ 0–23, min ∈ 0–59; вне диапазона time.Date нормализует значения
// молча — валидация ответственность вызывающего.
func New(hour, min int, fn func(context.Context) error) *Scheduler {
	return &Scheduler{
		hour:       hour,
		min:        min,
		fn:         fn,
		now:        time.Now,
		wait:       waitDur,
		retryEvery: time.Hour,
	}
}

// NextRun — ближайшие hour:min UTC. Используется статус-эндпоинтом.
func (s *Scheduler) NextRun() time.Time { return nextRun(s.now(), s.hour, s.min) }

// Run — блокирующий цикл: дождаться срока, вызвать fn. При ошибке fn
// повтор через retryEvery, при успехе — завтра в hour:min. Выход по ctx.
func (s *Scheduler) Run(ctx context.Context) {
	delay := s.initialDelay()
	for {
		if err := s.wait(ctx, delay); err != nil {
			return // ctx отменён — graceful shutdown
		}
		if err := s.fn(ctx); err != nil {
			if ctx.Err() == nil {
				slog.Warn("scheduler: запуск не удался, повтор позже", "err", err, "retryIn", s.retryEvery)
			}
			delay = s.retryEvery
		} else {
			delay = s.NextRun().Sub(s.now())
		}
	}
}

// initialDelay — сколько ждать первого запуска. Если сегодняшние hour:min
// уже прошли — 0: догон после рестарта (повтор безвреден для идемпотентных fn).
func (s *Scheduler) initialDelay() time.Duration {
	now := s.now().UTC()
	run := time.Date(now.Year(), now.Month(), now.Day(), s.hour, s.min, 0, 0, time.UTC)
	if !run.After(now) {
		return 0
	}
	return run.Sub(now)
}

// nextRun — ближайшие hour:min UTC относительно now: сегодня, если ещё не
// наступили, иначе завтра. Ровно hour:min считается наступившим.
func nextRun(now time.Time, hour, min int) time.Time {
	now = now.UTC()
	run := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, time.UTC)
	if !run.After(now) {
		run = run.AddDate(0, 0, 1)
	}
	return run
}

// waitDur — реальная реализация wait: спать d или выйти по ctx.
// d <= 0 — не спать (немедленный запуск), но отменённый ctx важнее.
func waitDur(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
