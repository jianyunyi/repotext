package control

import (
	"context"
	"crypto/sha256"
	"fmt"
	"repotest/internal/domain"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestConfirmPreviewCreatesOnePersistentTaskAndOutbox(t *testing.T) {
	s := authStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	tokenHash := sha256.Sum256([]byte("single-use-confirm-token"))
	preview := StoredPreview{RepoURL: "https://github.com/acme/repo", Ref: "main", CommitSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ExpiresAt: now.Add(10 * time.Minute)}
	if err := s.StorePreview(ctx, tokenHash[:], preview); err != nil {
		t.Fatal(err)
	}
	task := domain.Task{ID: "confirmed-task", Status: domain.StatusQueued, CreatedAt: now}
	identity := testExecutionIdentity()
	got, reused, err := s.ConfirmPreview(ctx, tokenHash[:], task, identity, now, 10*time.Minute)
	if err != nil || reused || got.ID != task.ID || got.RepoURL != preview.RepoURL || got.Ref != preview.Ref || got.Commit != preview.CommitSHA || got.Status != domain.StatusQueued {
		t.Fatalf("first confirm=%+v reused=%v err=%v", got, reused, err)
	}
	task.ID = "retry-task"
	got, reused, err = s.ConfirmPreview(ctx, tokenHash[:], task, identity, now, 10*time.Minute)
	if err != nil || !reused || got.ID != "confirmed-task" {
		t.Fatalf("idempotent confirm=%+v reused=%v err=%v", got, reused, err)
	}
	var tasks, outbox int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM outbox").Scan(&outbox); err != nil {
		t.Fatal(err)
	}
	if tasks != 1 || outbox != 1 {
		t.Fatalf("tasks=%d outbox=%d", tasks, outbox)
	}
}

func TestConfirmPreviewWithTokenRollsBackTaskWhenTokenPersistenceFails(t *testing.T) {
	s := authStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	previewHash := sha256.Sum256([]byte("atomic-confirm-preview"))
	controlHash := sha256.Sum256([]byte("atomic-control-token"))
	preview := StoredPreview{RepoURL: "https://github.com/acme/repo", Ref: "main", CommitSHA: strings.Repeat("a", 40), ExpiresAt: now.Add(10 * time.Minute)}
	if err := s.StorePreview(ctx, previewHash[:], preview); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("CREATE TRIGGER reject_task_token BEFORE INSERT ON task_tokens BEGIN SELECT RAISE(ABORT, 'token persistence rejected'); END"); err != nil {
		t.Fatal(err)
	}
	task := domain.Task{ID: "atomic-confirm-task", Status: domain.StatusQueued, CreatedAt: now}
	if _, _, err := s.ConfirmPreviewWithToken(ctx, previewHash[:], task, testExecutionIdentity(), controlHash[:], now, 10*time.Minute); err == nil {
		t.Fatal("confirmation succeeded despite token persistence failure")
	}
	for _, table := range []string{"tasks", "outbox", "confirmed_previews", "task_tokens"} {
		var count int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("%s contains %d rows after rollback", table, count)
		}
	}
	var previews int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM previews").Scan(&previews); err != nil {
		t.Fatal(err)
	}
	if previews != 1 {
		t.Fatalf("preview count after rollback=%d, want 1", previews)
	}
}

func TestConfirmPreviewRejectsExpiredOrMismatchedExecution(t *testing.T) {
	s := authStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, tc := range []struct {
		name      string
		commit    string
		expiresAt time.Time
	}{
		{name: "wrong commit", commit: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", expiresAt: now.Add(time.Minute)},
		{name: "expired", commit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", expiresAt: now.Add(-time.Second)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tokenHash := sha256.Sum256([]byte(tc.name))
			preview := StoredPreview{RepoURL: "https://github.com/acme/repo", Ref: "main", CommitSHA: tc.commit, ExpiresAt: tc.expiresAt}
			if err := s.StorePreview(ctx, tokenHash[:], preview); err != nil {
				t.Fatal(err)
			}
			task := domain.Task{ID: "rejected-" + strings.ReplaceAll(tc.name, " ", "-"), Status: domain.StatusQueued, CreatedAt: now}
			_, _, err := s.ConfirmPreview(ctx, tokenHash[:], task, testExecutionIdentity(), now, 10*time.Minute)
			if err == nil {
				t.Fatal("invalid preview confirmation succeeded")
			}
		})
	}
}

func TestWorkerStateAndReportCommitAreFencedByAttempt(t *testing.T) {
	s := authStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	task := domain.Task{ID: "worker-task", RepoURL: "https://github.com/acme/repo", Commit: strings.Repeat("a", 40), Status: domain.StatusQueued, CreatedAt: now}
	if _, _, err := s.CreateIdempotent(ctx, task, testExecutionIdentity(), now, 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	started, changed, err := s.StartTask(ctx, task.ID, now)
	if err != nil || !changed || started.Status != domain.StatusRunning || started.Attempt != 1 {
		t.Fatalf("start=%+v changed=%v err=%v", started, changed, err)
	}
	if ok, err := s.SetTaskPhase(ctx, task.ID, started.Attempt, domain.PhaseTest); err != nil || !ok {
		t.Fatalf("phase update=%v err=%v", ok, err)
	}
	report := []byte(`{"task_id":"worker-task","packages":[{"import_path":"example.test/pkg","status":"passed"}]}`)
	ok, err := s.CompleteTask(ctx, task.ID, started.Attempt, domain.StatusSucceeded, "", "", report, []byte("go test output"), now.Add(time.Second))
	if err != nil || !ok {
		t.Fatalf("complete=%v err=%v", ok, err)
	}
	if ok, err = s.CompleteTask(ctx, task.ID, started.Attempt-1, domain.StatusFailed, "stale", "stale", []byte(`{}`), nil, now.Add(2*time.Second)); err != nil || ok {
		t.Fatalf("stale completion=%v err=%v", ok, err)
	}
	gotReport, err := s.GetReport(ctx, task.ID)
	if err != nil || string(gotReport) != string(report) {
		t.Fatalf("report=%s err=%v", gotReport, err)
	}
	gotLog, err := s.GetLog(ctx, task.ID)
	if err != nil || string(gotLog) != "go test output" {
		t.Fatalf("log=%s err=%v", gotLog, err)
	}
	got, err := s.Get(ctx, task.ID)
	if err != nil || got.Status != domain.StatusSucceeded || got.Phase != domain.PhaseTest || got.FinishedAt == nil {
		t.Fatalf("task=%+v err=%v", got, err)
	}
}

func TestRecoverInterruptedRunRequeuesTaskAndOutbox(t *testing.T) {
	s := authStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	task := domain.Task{ID: "restart-task", RepoURL: "https://github.com/acme/repo", Commit: strings.Repeat("a", 40), Status: domain.StatusQueued, CreatedAt: now}
	if _, _, err := s.CreateIdempotent(ctx, task, testExecutionIdentity(), now, 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, changed, err := s.StartTask(ctx, task.ID, now); err != nil || !changed {
		t.Fatalf("start changed=%v err=%v", changed, err)
	}
	if _, err := s.ClaimOutbox(ctx, now, 10*time.Minute, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM outbox WHERE task_id=?", task.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := s.RecoverInterrupted(ctx, now.Add(time.Second)); err != nil || n != 1 {
		t.Fatalf("recovered=%d err=%v", n, err)
	}
	got, err := s.Get(ctx, task.ID)
	if err != nil || got.Status != domain.StatusQueued || got.Attempt != 1 || got.ErrorCode != "worker_interrupted" {
		t.Fatalf("recovered task=%+v err=%v", got, err)
	}
	items, err := s.ClaimOutbox(ctx, now.Add(time.Second), 10*time.Minute, 1)
	if err != nil || len(items) != 1 {
		t.Fatalf("outbox=%+v err=%v", items, err)
	}
}

func TestCreateIdempotentStoresOneTaskAndOutboxForConcurrentKey(t *testing.T) {
	s := authStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	start := make(chan struct{})
	var wg sync.WaitGroup
	ids := make(chan string, 24)
	errs := make(chan error, 24)
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			task := domain.Task{ID: fmt.Sprintf("task-%02d", i), RepoURL: "https://github.com/acme/repo", Commit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Status: domain.StatusQueued, CreatedAt: now}
			got, _, err := s.CreateIdempotent(ctx, task, testExecutionIdentity(), now, 10*time.Minute)
			if err != nil {
				errs <- err
				return
			}
			ids <- got.ID
		}(i)
	}
	close(start)
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var id string
	for got := range ids {
		if id == "" {
			id = got
		} else if id != got {
			t.Fatalf("duplicate key yielded different task IDs: %s / %s", id, got)
		}
	}
	var tasks, outbox int
	key, err := testExecutionIdentity().WorkKey()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM tasks WHERE work_key=?", key).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM outbox WHERE task_id=?", id).Scan(&outbox); err != nil {
		t.Fatal(err)
	}
	if tasks != 1 || outbox != 1 {
		t.Fatalf("tasks=%d outbox=%d", tasks, outbox)
	}
}

func TestCreateIdempotentUsesVersionsAndExpiresOnlyTerminalResults(t *testing.T) {
	s := authStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	window := 10 * time.Minute
	first := domain.Task{ID: "one", RepoURL: "https://github.com/acme/repo", Commit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Status: domain.StatusSucceeded, CreatedAt: now.Add(-11 * time.Minute)}
	got, reused, err := s.CreateIdempotent(ctx, first, testExecutionIdentity(), now, window)
	if err != nil || reused || got.ID != "one" {
		t.Fatalf("first got=%+v reused=%t err=%v", got, reused, err)
	}
	next := domain.Task{ID: "two", RepoURL: "https://github.com/acme/repo", Commit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Status: domain.StatusQueued, CreatedAt: now}
	got, reused, err = s.CreateIdempotent(ctx, next, testExecutionIdentity(), now, window)
	if err != nil || reused || got.ID != "two" {
		t.Fatalf("expired terminal result not replaced: got=%+v reused=%t err=%v", got, reused, err)
	}
	different := domain.Task{ID: "three", RepoURL: "https://github.com/acme/repo", Commit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Status: domain.StatusQueued, CreatedAt: now}
	got, reused, err = s.CreateIdempotent(ctx, different, ExecutionIdentity{repoURL: "https://github.com/acme/repo", commitSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", goVersion: "go1.24", runnerVersion: "runner-v1", config: []byte("cfg")}, now, window)
	if err != nil || reused || got.ID != "three" {
		t.Fatalf("versioned key collided: got=%+v reused=%t err=%v", got, reused, err)
	}
}

func TestOutboxClaimLeaseRetryAndRestart(t *testing.T) {
	path := t.TempDir() + "/outbox.db"
	ctx := context.Background()
	s, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	task := domain.Task{ID: "outbox-task", RepoURL: "https://github.com/acme/repo", Commit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Status: domain.StatusQueued, CreatedAt: now}
	if _, _, err = s.CreateIdempotent(ctx, task, testExecutionIdentity(), now, 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	items, err := s.ClaimOutbox(ctx, now, 5*time.Minute, 10)
	if err != nil || len(items) != 1 || items[0].TaskID != "outbox-task" {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	if items, err = s.ClaimOutbox(ctx, now.Add(time.Minute), 5*time.Minute, 10); err != nil || len(items) != 0 {
		t.Fatalf("leased item reclaimed early: items=%+v err=%v", items, err)
	}
	items, err = s.ClaimOutbox(ctx, now.Add(6*time.Minute), 5*time.Minute, 10)
	if err != nil || len(items) != 1 || items[0].Attempts != 2 {
		t.Fatalf("expired lease not retried: items=%+v err=%v", items, err)
	}
	if err = s.AckOutbox(ctx, items[0].ID); err != nil {
		t.Fatal(err)
	}
	items, err = s.ClaimOutbox(ctx, now.Add(7*time.Minute), 5*time.Minute, 10)
	if err != nil || len(items) != 0 {
		t.Fatalf("acked item returned: items=%+v err=%v", items, err)
	}
}

func TestCreateIdempotentReusesFromFinishAndPreservesHistory(t *testing.T) {
	s := authStore(t)
	ctx := context.Background()
	finished := time.Now().UTC().Truncate(time.Second)
	identity := testExecutionIdentity()
	first := domain.Task{ID: "original", RepoURL: identity.repoURL, Commit: identity.commitSHA, Status: domain.StatusSucceeded, CreatedAt: finished.Add(-30 * time.Minute), FinishedAt: &finished}
	if _, _, err := s.CreateIdempotent(ctx, first, identity, finished, 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := s.PutReport(ctx, first.ID, []byte(`{"packages":[]}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.PutLog(ctx, first.ID, []byte("original log")); err != nil {
		t.Fatal(err)
	}
	next := domain.Task{ID: "replacement", RepoURL: identity.repoURL, Commit: identity.commitSHA, Status: domain.StatusQueued}
	got, reused, err := s.CreateIdempotent(ctx, next, identity, finished.Add(9*time.Minute), 10*time.Minute)
	if err != nil || !reused || got.ID != first.ID {
		t.Fatalf("before expiry: id=%s reused=%v err=%v", got.ID, reused, err)
	}
	got, reused, err = s.CreateIdempotent(ctx, next, identity, finished.Add(10*time.Minute), 10*time.Minute)
	if err != nil || reused || got.ID != next.ID {
		t.Fatalf("at expiry: id=%s reused=%v err=%v", got.ID, reused, err)
	}
	if _, err := s.GetReport(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetLog(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
}

func testExecutionIdentity() ExecutionIdentity {
	return ExecutionIdentity{repoURL: "https://github.com/acme/repo", commitSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", goVersion: "go1.23", runnerVersion: "runner-v1", config: []byte("cfg")}
}

func TestCreateIdempotentRejectsMismatchedTaskIdentity(t *testing.T) {
	s := authStore(t)
	task := domain.Task{ID: "mismatch", RepoURL: "https://github.com/attacker/other", Commit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Status: domain.StatusQueued}
	if _, _, err := s.CreateIdempotent(context.Background(), task, testExecutionIdentity(), time.Now(), time.Minute); err == nil {
		t.Fatal("mismatched task identity accepted")
	}
	var tasks, outbox int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM outbox").Scan(&outbox); err != nil {
		t.Fatal(err)
	}
	if tasks != 0 || outbox != 0 {
		t.Fatalf("mismatch created task=%d outbox=%d", tasks, outbox)
	}
}
