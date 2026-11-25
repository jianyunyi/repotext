package control

import (
	"context"
	"errors"
	"repotest/internal/domain"
	"sync"
	"time"
)

var ErrNotFound = errors.New("task not found")

type Store interface {
	Create(context.Context, domain.Task) error
	Get(context.Context, string) (domain.Task, error)
	Transition(context.Context, string, domain.Status, domain.Status, string) (bool, error)
	Delete(context.Context, string) error
}
type MemoryStore struct {
	mu    sync.RWMutex
	tasks map[string]domain.Task
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{tasks: make(map[string]domain.Task)} }
func (s *MemoryStore) Create(_ context.Context, t domain.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[t.ID] = t
	return nil
}
func (s *MemoryStore) Get(_ context.Context, id string) (domain.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[id]
	if !ok {
		return t, ErrNotFound
	}
	return t, nil
}
func (s *MemoryStore) Transition(_ context.Context, id string, from, to domain.Status, code string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return false, ErrNotFound
	}
	if t.Status != from || !domain.CanTransition(from, to) {
		return false, nil
	}
	t.Status = to
	t.ErrorCode = code
	now := time.Now().UTC()
	if to == domain.StatusRunning {
		t.StartedAt = &now
	}
	if to != domain.StatusQueued && to != domain.StatusRunning {
		t.FinishedAt = &now
	}
	s.tasks[id] = t
	return true, nil
}

func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tasks[id]; !ok {
		return ErrNotFound
	}
	delete(s.tasks, id)
	return nil
}
