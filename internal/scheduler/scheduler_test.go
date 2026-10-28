// Тесты планировщика: фейковые часы (now) и ручной «срок» (wait) —
// без реального сна, детерминированно.
package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestNextRun(t *testing.T) {
	cases := []struct{ name, now, want string }{
		{"до времени — сегодня", "2026-10-28T10:00:00Z", "2026-10-28T15:00:00Z"},
		{"после времени — завтра", "2026-10-28T16:30:00Z", "2026-10-29T15:00:00Z"},
		{"ровно время — завтра", "2026-10-28T15:00:00Z", "2026-10-29T15:00:00Z"},
		{"перед полуночью", "2026-10-28T23:59:59Z", "2026-10-29T15:00:00Z"},
		{"сразу после полуночи", "2026-10-28T00:00:00Z", "2026-10-28T15:00:00Z"},
	}
	for _, tc := range cases {
		got := nextRun(mustTime(t, tc.now), 15, 0)
		if !got.Equal(mustTime(t, tc.want)) {
			t.Fatalf("%s: got %v, want %s", tc.name, got.UTC(), tc.want)
		}
	}
}

// fakeClock — фейковые часы с ручным сроком: wait пишет задержку в delays
// и ждёт tick() или отмены ctx.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	delays []time.Duration
	fire   chan struct{}
}

func newFakeClock(t *testing.T, now string) *fakeClock {
	return &fakeClock{now: mustTime(t, now), fire: make(chan struct{})}
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) wait(ctx context.Context, d time.Duration) error {
	f.mu.Lock()
	f.delays = append(f.delays, d)
	f.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-f.fire:
		return nil
	}
}

func (f *fakeClock) tick() { f.fire <- struct{}{} }

// delayN — n-я записанная задержка (нумерация с 1); ждёт её появления
// до 2 с — Run работает в горутине, задержка появляется асинхронно.
func (f *fakeClock) delayN(t *testing.T, n int) time.Duration {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		f.mu.Lock()
		var d time.Duration
		enough := len(f.delays) >= n
		if enough {
			d = f.delays[n-1]
		}
		f.mu.Unlock()
		if enough {
			return d
		}
		if time.Now().After(deadline) {
			t.Fatalf("задержка #%d так и не записана", n)
		}
		time.Sleep(time.Millisecond)
	}
}

// Старт после сегодняшних 15:00 — немедленный запуск (догон после рестарта),
// после успеха — завтрашние 15:00 (от 16:00 это 23 ч).
func TestRunCatchUpThenDaily(t *testing.T) {
	clk := newFakeClock(t, "2026-10-28T16:00:00Z")
	calls := make(chan struct{}, 4)
	s := New(15, 0, func(context.Context) error { calls <- struct{}{}; return nil })
	s.now, s.wait = clk.Now, clk.wait

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	clk.tick()
	<-calls
	if d := clk.delayN(t, 1); d != 0 {
		t.Fatalf("догон: задержка = %v, хотим 0", d)
	}
	if d := clk.delayN(t, 2); d != 23*time.Hour {
		t.Fatalf("после успеха: задержка = %v, хотим 23ч", d)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run не завершился по отмене ctx")
	}
}

// Старт до 15:00 — ждём ровно до 15:00 (от 14:00 это 1 ч), fn не зовём.
func TestRunWaitsUntilTodayTime(t *testing.T) {
	clk := newFakeClock(t, "2026-10-28T14:00:00Z")
	calls := make(chan struct{}, 1)
	s := New(15, 0, func(context.Context) error { calls <- struct{}{}; return nil })
	s.now, s.wait = clk.Now, clk.wait

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	if d := clk.delayN(t, 1); d != time.Hour {
		t.Fatalf("задержка = %v, хотим 1ч", d)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run не завершился по отмене ctx")
	}
	select {
	case <-calls:
		t.Fatal("fn вызван без tick")
	default:
	}
}

// Ошибка fn — повтор через retryEvery; успех после повтора — возврат
// к суточному ритму.
func TestRunRetryOnError(t *testing.T) {
	clk := newFakeClock(t, "2026-10-28T16:00:00Z")
	var fail atomic.Bool
	fail.Store(true)
	calls := make(chan struct{}, 4)
	s := New(15, 0, func(context.Context) error {
		calls <- struct{}{}
		if fail.Load() {
			return errors.New("архив недоступен")
		}
		return nil
	})
	s.now, s.wait, s.retryEvery = clk.Now, clk.wait, time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	clk.tick()
	<-calls
	if d := clk.delayN(t, 2); d != time.Hour {
		t.Fatalf("после ошибки: задержка = %v, хотим 1ч (retryEvery)", d)
	}
	fail.Store(false)
	clk.tick()
	<-calls
	if d := clk.delayN(t, 3); d != 23*time.Hour {
		t.Fatalf("после успеха: задержка = %v, хотим 23ч", d)
	}
}
