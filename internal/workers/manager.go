package workers

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

type Task struct {
	Name     string
	Interval time.Duration
	Run      func(context.Context) error
}
type Manager struct {
	log   *slog.Logger
	tasks []Task
	wg    sync.WaitGroup
}

func NewManager(log *slog.Logger, tasks ...Task) *Manager { return &Manager{log: log, tasks: tasks} }
func (m *Manager) Names() []string {
	out := make([]string, len(m.tasks))
	for i, t := range m.tasks {
		out[i] = t.Name
	}
	return out
}
func (m *Manager) Start(ctx context.Context) {
	for _, task := range m.tasks {
		task := task
		m.wg.Add(1)
		go func() { defer m.wg.Done(); m.run(ctx, task) }()
	}
}
func (m *Manager) Wait() { m.wg.Wait() }
func (m *Manager) run(ctx context.Context, t Task) {
	run := func() {
		if err := t.Run(ctx); err != nil && !errors.Is(err, context.Canceled) && m.log != nil {
			m.log.Error("worker iteration failed", slog.String("worker", t.Name), slog.String("error", err.Error()))
		}
	}
	run()
	ticker := time.NewTicker(t.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
