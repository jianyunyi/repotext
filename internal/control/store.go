package control

import (
	"context"
	"errors"
	"repotest/internal/domain"
	"sync"
	"time"
)

var ErrNotFound = errors.New("task not found")

type TaskTokenStore interface {
	StoreTaskToken(context.Context, string, []byte) error
	GetTaskTokenHash(context.Context, string) ([]byte, error)
}

type Store interface {
	Create(context.Context, domain.Task) error
	Get(context.Context, string) (domain.Task, error)
	Transition(context.Context, string, domain.Status, domain.Status, string) (bool, error)
	Delete(context.Context, string) error
}

type MemoryStore struct {
	mu          sync.RWMutex
	tasks       map[string]domain.Task
	tokenHashes map[string][]byte
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{tasks: make(map[string]domain.Task), tokenHashes: make(map[string][]byte)}
}

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
	delete(s.tokenHashes, id)
	return nil
}

func (s *MemoryStore) StoreTaskToken(_ context.Context, taskID string, hash []byte) error {
	if taskID == "" || len(hash) == 0 {
		return errors.New("invalid task token")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokenHashes[taskID] = append([]byte(nil), hash...)
	return nil
}

func (s *MemoryStore) GetTaskTokenHash(_ context.Context, taskID string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	hash, ok := s.tokenHashes[taskID]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), hash...), nil
}
