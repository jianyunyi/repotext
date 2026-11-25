package control

import (
	"context"
	"errors"
	"sync"
)

var ErrTaskProcessNotRegistered = errors.New("task process is not registered")
var ErrTaskProcessAlreadyRegistered = errors.New("task process is already registered")

type TaskProcesses interface {
	CancelAndWait(context.Context, string) error
}

type processHandle struct {
	cancel context.CancelFunc
	done   <-chan struct{}
}
type TaskProcessRegistry struct {
	mu        sync.Mutex
	processes map[string]processHandle
}

func NewTaskProcessRegistry() *TaskProcessRegistry {
	return &TaskProcessRegistry{processes: make(map[string]processHandle)}
}

func (r *TaskProcessRegistry) Register(taskID string, cancel context.CancelFunc, done <-chan struct{}) (func(), error) {
	if taskID == "" || cancel == nil || done == nil {
		return nil, errors.New("invalid task process registration")
	}
	h := processHandle{cancel: cancel, done: done}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.processes[taskID]; ok {
		return nil, ErrTaskProcessAlreadyRegistered
	}
	r.processes[taskID] = h
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if current, ok := r.processes[taskID]; ok && current.done == done {
			delete(r.processes, taskID)
		}
	}, nil
}

func (r *TaskProcessRegistry) CancelAndWait(ctx context.Context, taskID string) error {
	r.mu.Lock()
	h, ok := r.processes[taskID]
	r.mu.Unlock()
	if !ok {
		return ErrTaskProcessNotRegistered
	}
	h.cancel()
	select {
	case <-h.done:
		r.mu.Lock()
		if current, exists := r.processes[taskID]; exists && current.done == h.done {
			delete(r.processes, taskID)
		}
		r.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (r *TaskProcessRegistry) Run(ctx context.Context, taskID string, run func(context.Context) error) error {
	if run == nil {
		return errors.New("task process runner is nil")
	}
	taskCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	unregister, err := r.Register(taskID, cancel, done)
	if err != nil {
		cancel()
		return err
	}
	defer unregister()
	defer close(done)
	defer cancel()
	return run(taskCtx)
}
