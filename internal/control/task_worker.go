package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"repotest/internal/domain"
)

type TaskExecution struct {
	ExitCode  int
	TimedOut  bool
	Output    []byte
	Truncated bool
	Report    domain.Report
}

type TaskExecutor interface {
	Execute(context.Context, domain.Task, func(domain.Phase) error) (TaskExecution, error)
}

type TaskWorker struct {
	Store       *SQLiteStore
	Executor    TaskExecutor
	Processes   *TaskProcessRegistry
	Now         func() time.Time
	OnPhase     func(string, domain.Phase)
	PollEvery   time.Duration
	Lease       time.Duration
	MaxAttempts int
}

func (w TaskWorker) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}

func (w TaskWorker) ProcessOne(ctx context.Context) (bool, error) {
	if w.Store == nil || w.Executor == nil {
		return false, errors.New("task worker requires storage and executor")
	}
	lease := w.Lease
	if lease <= 0 {
		lease = 12 * time.Minute
	}
	items, err := w.Store.ClaimOutbox(ctx, w.now(), lease, 1)
	if err != nil || len(items) == 0 {
		return false, err
	}
	item := items[0]
	task, err := w.Store.Get(ctx, item.TaskID)
	if errors.Is(err, ErrNotFound) {
		return true, w.Store.AckOutbox(ctx, item.ID)
	}
	if err != nil {
		return true, err
	}
	if task.Status == domain.StatusRunning {
		return true, nil
	}
	if task.Status != domain.StatusQueued {
		return true, w.Store.AckOutbox(ctx, item.ID)
	}
	task, started, err := w.Store.StartTask(ctx, task.ID, w.now())
	if err != nil {
		return true, err
	}
	if !started {
		return true, w.Store.AckOutbox(ctx, item.ID)
	}
	execute := func(taskCtx context.Context) error {
		progress := func(phase domain.Phase) error {
			if taskCtx.Err() != nil {
				return taskCtx.Err()
			}
			changed, phaseErr := w.Store.SetTaskPhase(taskCtx, task.ID, task.Attempt, phase)
			if phaseErr != nil {
				return phaseErr
			}
			if !changed {
				return errors.New("task phase update lost its attempt")
			}
			if w.OnPhase != nil {
				w.OnPhase(task.ID, phase)
			}
			return nil
		}
		result, execErr := w.Executor.Execute(taskCtx, task, progress)
		return w.finish(ctx, task, item.ID, result, execErr)
	}
	if w.Processes == nil {
		return true, execute(ctx)
	}
	return true, w.Processes.Run(ctx, task.ID, func(taskCtx context.Context) error {
		return execute(taskCtx)
	})
}

func (w TaskWorker) finish(parent context.Context, task domain.Task, outboxID int64, result TaskExecution, execErr error) error {
	now := w.now()
	if parent.Err() != nil && errors.Is(execErr, context.Canceled) {
		if len(result.Output) > 0 {
			if err := w.Store.PutLog(context.Background(), task.ID, result.Output); err != nil {
				return errors.Join(parent.Err(), err)
			}
		}
		_, err := w.Store.RetryTask(context.Background(), task.ID, task.Attempt, "worker_shutdown", "服务关闭时中断，将在下次启动后恢复。", now, false, now)
		return errors.Join(parent.Err(), err)
	}
	if errors.Is(execErr, context.Canceled) {
		_, err := w.Store.CompleteTask(context.Background(), task.ID, task.Attempt, domain.StatusCancelled, "", "", nil, result.Output, now)
		if err != nil {
			return err
		}
		return w.Store.AckOutbox(context.Background(), outboxID)
	}
	if errors.Is(execErr, context.DeadlineExceeded) || result.TimedOut {
		return w.finishResult(task, outboxID, result, domain.StatusTimeout, "execution_timeout", "测试执行超过时间限制。", now)
	}
	if execErr != nil {
		maxAttempts := w.MaxAttempts
		if maxAttempts <= 0 {
			maxAttempts = 3
		}
		if task.Attempt >= maxAttempts {
			return w.finishResult(task, outboxID, result, domain.StatusError, "worker_failed", "隔离执行失败，已达到重试上限。", now)
		}
		if len(result.Output) > 0 {
			if err := w.Store.PutLog(context.Background(), task.ID, result.Output); err != nil {
				return err
			}
		}
		delay := time.Duration(1<<(task.Attempt-1)) * time.Second
		if delay > time.Minute {
			delay = time.Minute
		}
		_, err := w.Store.RetryTask(context.Background(), task.ID, task.Attempt, "worker_retry", "隔离执行暂时失败，将自动重试。", now.Add(delay), false, now)
		return err
	}
	status := domain.StatusSucceeded
	if result.ExitCode != 0 {
		status = domain.StatusFailed
	}
	return w.finishResult(task, outboxID, result, status, "", "", now)
}

func (w TaskWorker) finishResult(task domain.Task, outboxID int64, result TaskExecution, status domain.Status, code, message string, now time.Time) error {
	result.Report.TaskID = task.ID
	result.Report.Truncated = result.Truncated
	report, err := json.Marshal(result.Report)
	if err != nil {
		return err
	}
	changed, err := w.Store.CompleteTask(context.Background(), task.ID, task.Attempt, status, code, message, report, result.Output, now)
	if err != nil {
		return err
	}
	if !changed {
		current, getErr := w.Store.Get(context.Background(), task.ID)
		if getErr != nil {
			return getErr
		}
		if current.Status == domain.StatusRunning {
			return fmt.Errorf("task %s completion lost attempt %d", task.ID, task.Attempt)
		}
	}
	return w.Store.AckOutbox(context.Background(), outboxID)
}

func (w TaskWorker) Run(ctx context.Context) error {
	poll := w.PollEvery
	if poll <= 0 {
		poll = 500 * time.Millisecond
	}
	for {
		worked, err := w.ProcessOne(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		delay := poll
		if err != nil {
			delay = time.Second
		} else if worked {
			continue
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
