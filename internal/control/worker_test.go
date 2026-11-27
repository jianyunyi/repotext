package control

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"repotest/internal/runner"
	"strings"
	"testing"
	"time"

	"repotest/internal/domain"
)

type testTaskExecutor func(context.Context, domain.Task, func(domain.Phase) error) (TaskExecution, error)

func (f testTaskExecutor) Execute(ctx context.Context, task domain.Task, phase func(domain.Phase) error) (TaskExecution, error) {
	return f(ctx, task, phase)
}

func TestTaskWorkerConsumesOutboxAndPersistsReport(t *testing.T) {
	store := authStore(t)
	now := time.Now().UTC()
	task := domain.Task{ID: "worker-success", RepoURL: "https://github.com/acme/repo", Commit: strings.Repeat("a", 40), Status: domain.StatusQueued, CreatedAt: now}
	if _, _, err := store.CreateIdempotent(context.Background(), task, testExecutionIdentity(), now, 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	var phases []domain.Phase
	worker := TaskWorker{Store: store, Executor: testTaskExecutor(func(_ context.Context, task domain.Task, setPhase func(domain.Phase) error) (TaskExecution, error) {
		if err := setPhase(domain.PhaseDeps); err != nil {
			return TaskExecution{}, err
		}
		if err := setPhase(domain.PhaseTest); err != nil {
			return TaskExecution{}, err
		}
		return TaskExecution{ExitCode: 0, Output: []byte("go test passed"), Report: domain.Report{TaskID: task.ID, Packages: []domain.Package{{ImportPath: "example.test/pkg", Status: "passed"}}}}, nil
	}), Processes: NewTaskProcessRegistry(), Now: func() time.Time { return now.Add(time.Second) }}
	worker.OnPhase = func(_ string, phase domain.Phase) { phases = append(phases, phase) }
	worked, err := worker.ProcessOne(context.Background())
	if err != nil || !worked {
		t.Fatalf("worked=%v err=%v", worked, err)
	}
	got, err := store.Get(context.Background(), task.ID)
	if err != nil || got.Status != domain.StatusSucceeded || got.Attempt != 1 || got.Phase != domain.PhaseTest {
		t.Fatalf("task=%+v err=%v", got, err)
	}
	if len(phases) != 2 || phases[0] != domain.PhaseDeps || phases[1] != domain.PhaseTest {
		t.Fatalf("phases=%v", phases)
	}
	report, err := store.GetReport(context.Background(), task.ID)
	if err != nil || !strings.Contains(string(report), `"status":"passed"`) {
		t.Fatalf("report=%s err=%v", report, err)
	}
	logBody, err := store.GetLog(context.Background(), task.ID)
	if err != nil || string(logBody) != "go test passed" {
		t.Fatalf("log=%s err=%v", logBody, err)
	}
	req := loopbackRequest(httptest.NewRequest(http.MethodGet, "/api/v1/tasks/worker-success/report", nil))
	response := httptest.NewRecorder()
	(API{Store: store, CSRFToken: testCSRFToken}).Handler().ServeHTTP(response, req)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"passed"`) {
		t.Fatalf("HTTP report status=%d body=%s", response.Code, response.Body.String())
	}
	items, err := store.ClaimOutbox(context.Background(), now.Add(2*time.Second), time.Minute, 1)
	if err != nil || len(items) != 0 {
		t.Fatalf("remaining outbox=%+v err=%v", items, err)
	}
}

func TestTaskWorkerCancellationWaitsAndPersistsCancelledStatus(t *testing.T) {
	store := authStore(t)
	now := time.Now().UTC()
	task := domain.Task{ID: "worker-cancel", RepoURL: "https://github.com/acme/repo", Commit: strings.Repeat("a", 40), Status: domain.StatusQueued, CreatedAt: now}
	if _, _, err := store.CreateIdempotent(context.Background(), task, testExecutionIdentity(), now, 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	registry := NewTaskProcessRegistry()
	worker := TaskWorker{Store: store, Processes: registry, Executor: testTaskExecutor(func(ctx context.Context, task domain.Task, _ func(domain.Phase) error) (TaskExecution, error) {
		<-ctx.Done()
		return TaskExecution{Report: domain.Report{TaskID: task.ID}}, ctx.Err()
	})}
	done := make(chan error, 1)
	go func() {
		_, err := worker.ProcessOne(context.Background())
		done <- err
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		current, err := store.Get(context.Background(), task.ID)
		if err == nil && current.Status == domain.StatusRunning {
			break
		}
		time.Sleep(time.Millisecond * 10)
	}
	if err := registry.CancelAndWait(context.Background(), task.ID); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), task.ID)
	if err != nil || got.Status != domain.StatusCancelled {
		t.Fatalf("task=%+v err=%v", got, err)
	}
}

func TestTaskWorkerRequeuesOnServiceShutdown(t *testing.T) {
	store := authStore(t)
	now := time.Now().UTC()
	task := domain.Task{ID: "worker-shutdown", RepoURL: "https://github.com/acme/repo", Commit: strings.Repeat("a", 40), Status: domain.StatusQueued, CreatedAt: now}
	if _, _, err := store.CreateIdempotent(context.Background(), task, testExecutionIdentity(), now, 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	worker := TaskWorker{Store: store, Processes: NewTaskProcessRegistry(), Executor: testTaskExecutor(func(ctx context.Context, _ domain.Task, _ func(domain.Phase) error) (TaskExecution, error) {
		cancel()
		<-ctx.Done()
		return TaskExecution{}, ctx.Err()
	})}
	_, err := worker.ProcessOne(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("shutdown error=%v", err)
	}
	got, err := store.Get(context.Background(), task.ID)
	if err != nil || got.Status != domain.StatusQueued || got.ErrorCode != "worker_shutdown" {
		t.Fatalf("task=%+v err=%v", got, err)
	}
}

func TestTaskWorkerPersistsFailureReportAndLogsAtRetryLimit(t *testing.T) {
	store := authStore(t)
	now := time.Now().UTC()
	task := domain.Task{ID: "worker-error", RepoURL: "https://github.com/acme/repo", Commit: strings.Repeat("a", 40), Status: domain.StatusQueued, CreatedAt: now}
	if _, _, err := store.CreateIdempotent(context.Background(), task, testExecutionIdentity(), now, 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	worker := TaskWorker{Store: store, MaxAttempts: 1, Now: func() time.Time { return now.Add(time.Second) }, Executor: testTaskExecutor(func(_ context.Context, task domain.Task, _ func(domain.Phase) error) (TaskExecution, error) {
		return TaskExecution{Output: []byte("dependency fetch failed"), Report: domain.Report{TaskID: task.ID, Packages: []domain.Package{}}}, errors.New("dependency fetch failed")
	})}
	worked, err := worker.ProcessOne(context.Background())
	if err != nil || !worked {
		t.Fatalf("worked=%v err=%v", worked, err)
	}
	got, err := store.Get(context.Background(), task.ID)
	if err != nil || got.Status != domain.StatusError || got.ErrorCode != "worker_failed" {
		t.Fatalf("task=%+v err=%v", got, err)
	}
	if report, err := store.GetReport(context.Background(), task.ID); err != nil || !strings.Contains(string(report), `"task_id":"worker-error"`) {
		t.Fatalf("report=%s err=%v", report, err)
	}
	if logBody, err := store.GetLog(context.Background(), task.ID); err != nil || string(logBody) != "dependency fetch failed" {
		t.Fatalf("log=%s err=%v", logBody, err)
	}
}

func TestTaskWorkerRejectsExecutionIdentityMismatchBeforeExecutor(t *testing.T) {
	store := authStore(t)
	now := time.Now().UTC()
	task := domain.Task{ID: "identity-mismatch", RepoURL: "https://github.com/acme/repo", Commit: strings.Repeat("a", 40), Status: domain.StatusQueued, CreatedAt: now}
	if _, _, err := store.CreateIdempotent(context.Background(), task, testExecutionIdentity(), now, time.Minute); err != nil {
		t.Fatal(err)
	}
	called := false
	worker := TaskWorker{
		Store:    store,
		Identity: NewExecutionIdentity("", "", "go1.24", "runner-v1", []byte("cfg")),
		Executor: testTaskExecutor(func(context.Context, domain.Task, func(domain.Phase) error) (TaskExecution, error) {
			called = true
			return TaskExecution{}, nil
		}),
	}
	if _, err := worker.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if called || got.Status != domain.StatusError || got.ErrorCode != "execution_identity_mismatch" {
		t.Fatalf("called=%v task=%+v", called, got)
	}
}

func TestTaskWorkerAcceptsGlobalIdentityWithoutRepoOrCommit(t *testing.T) {
	store := authStore(t)
	now := time.Now().UTC()
	task := domain.Task{ID: "global-identity-match", RepoURL: "https://github.com/acme/repo", Commit: strings.Repeat("a", 40), Status: domain.StatusQueued, CreatedAt: now}
	identity := testExecutionIdentity()
	if _, _, err := store.CreateIdempotent(context.Background(), task, identity, now, time.Minute); err != nil {
		t.Fatal(err)
	}
	called := false
	worker := TaskWorker{
		Store:    store,
		Identity: NewExecutionIdentity("", "", identity.goVersion, identity.runnerVersion, identity.config),
		Executor: testTaskExecutor(func(context.Context, domain.Task, func(domain.Phase) error) (TaskExecution, error) {
			called = true
			return TaskExecution{Report: domain.Report{TaskID: task.ID, Packages: []domain.Package{}}}, nil
		}),
	}
	if _, err := worker.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), task.ID)
	if err != nil || !called || got.Status != domain.StatusSucceeded {
		t.Fatalf("called=%v task=%+v err=%v", called, got, err)
	}
}

func TestTaskWorkerPreservesTaskWhenCleanupFailsDuringCancel(t *testing.T) {
	store := authStore(t)
	now := time.Now().UTC()
	task := domain.Task{ID: "cleanup-pending", RepoURL: "https://github.com/acme/repo", Commit: strings.Repeat("a", 40), Status: domain.StatusQueued, CreatedAt: now}
	if _, _, err := store.CreateIdempotent(context.Background(), task, testExecutionIdentity(), now, time.Minute); err != nil {
		t.Fatal(err)
	}
	registry := NewTaskProcessRegistry()
	worker := TaskWorker{Store: store, Processes: registry, Executor: testTaskExecutor(func(ctx context.Context, task domain.Task, _ func(domain.Phase) error) (TaskExecution, error) {
		<-ctx.Done()
		return TaskExecution{}, errors.Join(ctx.Err(), runner.CleanupError{Err: errors.New("container removal failed")})
	})}
	done := make(chan error, 1)
	go func() {
		_, err := worker.ProcessOne(context.Background())
		done <- err
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, err := store.Get(context.Background(), task.ID)
		if err == nil && got.Status == domain.StatusRunning {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := registry.CancelAndWait(context.Background(), task.ID); !errors.Is(err, ErrCleanupPending) {
		t.Fatalf("cancel error=%v", err)
	}
	if err := <-done; !errors.Is(err, ErrCleanupPending) {
		t.Fatalf("worker error=%v", err)
	}
	got, err := store.Get(context.Background(), task.ID)
	if err != nil || got.Status != domain.StatusError || got.ErrorCode != "cleanup_pending" {
		t.Fatalf("task=%+v err=%v", got, err)
	}
}
