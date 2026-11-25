package control

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	_ "modernc.org/sqlite"
	"os"
	"repotest/internal/domain"
	"repotest/internal/runner"
	"strings"
	"time"
)

type SQLiteStore struct{ db *sql.DB }

type StoredPreview struct {
	RepoURL   string
	Ref       string
	CommitSHA string
	ExpiresAt time.Time
}

var ErrPreviewInvalid = errors.New("preview is invalid or expired")

func OpenSQLiteStore(path string) (*SQLiteStore, error) {
	db, e := sql.Open("sqlite", path)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	for _, q := range []string{"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON", "CREATE TABLE IF NOT EXISTS tasks(id TEXT PRIMARY KEY,status TEXT NOT NULL,task_json BLOB NOT NULL,created_at INTEGER NOT NULL,finished_at INTEGER,work_key TEXT)", "CREATE INDEX IF NOT EXISTS tasks_status_created ON tasks(status,created_at)", "CREATE TABLE IF NOT EXISTS sessions(token_hash BLOB PRIMARY KEY,expires_at INTEGER NOT NULL,revoked INTEGER NOT NULL DEFAULT 0)", "CREATE TABLE IF NOT EXISTS previews(token_hash BLOB PRIMARY KEY,repo_url TEXT NOT NULL,ref TEXT NOT NULL,commit_sha TEXT NOT NULL,expires_at INTEGER NOT NULL)", "CREATE TABLE IF NOT EXISTS confirmed_previews(token_hash BLOB PRIMARY KEY,task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,expires_at INTEGER NOT NULL)", "CREATE TABLE IF NOT EXISTS reports(task_id TEXT PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,body BLOB NOT NULL)", "CREATE TABLE IF NOT EXISTS logs(task_id TEXT PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,body BLOB NOT NULL)"} {
		if _, e = db.Exec(q); e != nil {
			db.Close()
			return nil, e
		}
	}
	if e = ensureTaskWorkKeyColumn(db); e != nil {
		db.Close()
		return nil, e
	}
	for _, q := range []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS tasks_work_key ON tasks(work_key) WHERE work_key IS NOT NULL",
		"CREATE TABLE IF NOT EXISTS outbox(id INTEGER PRIMARY KEY AUTOINCREMENT,task_id TEXT NOT NULL UNIQUE REFERENCES tasks(id) ON DELETE CASCADE,created_at INTEGER NOT NULL,available_at INTEGER NOT NULL,claimed_until INTEGER NOT NULL DEFAULT 0,attempts INTEGER NOT NULL DEFAULT 0)",
		"CREATE INDEX IF NOT EXISTS outbox_ready ON outbox(available_at,claimed_until,id)",
	} {
		if _, e = db.Exec(q); e != nil {
			db.Close()
			return nil, e
		}
	}
	if e = os.Chmod(path, 0600); e != nil {
		db.Close()
		return nil, e
	}
	return &SQLiteStore{db}, nil
}
func (s *SQLiteStore) Close() error { return s.db.Close() }
func (s *SQLiteStore) Create(c context.Context, t domain.Task) error {
	b, e := json.Marshal(t)
	if e != nil {
		return e
	}
	_, e = s.db.ExecContext(c, "INSERT INTO tasks(id,status,task_json,created_at) VALUES(?,?,?,?)", t.ID, t.Status, b, t.CreatedAt.Unix())
	return e
}
func (s *SQLiteStore) Get(c context.Context, id string) (domain.Task, error) {
	var b []byte
	e := s.db.QueryRowContext(c, "SELECT task_json FROM tasks WHERE id=?", id).Scan(&b)
	if errors.Is(e, sql.ErrNoRows) {
		return domain.Task{}, ErrNotFound
	}
	if e != nil {
		return domain.Task{}, e
	}
	var t domain.Task
	e = json.Unmarshal(b, &t)
	return t, e
}
func (s *SQLiteStore) Transition(c context.Context, id string, from, to domain.Status, code string) (bool, error) {
	tx, e := s.db.BeginTx(c, nil)
	if e != nil {
		return false, e
	}
	defer tx.Rollback()
	var b []byte
	if e = tx.QueryRowContext(c, "SELECT task_json FROM tasks WHERE id=? AND status=?", id, from).Scan(&b); errors.Is(e, sql.ErrNoRows) {
		return false, nil
	} else if e != nil {
		return false, e
	}
	var t domain.Task
	if e = json.Unmarshal(b, &t); e != nil {
		return false, e
	}
	if !domain.CanTransition(from, to) {
		return false, nil
	}
	t.Status = to
	t.ErrorCode = code
	now := time.Now().UTC()
	if to == domain.StatusRunning {
		t.StartedAt = &now
	}
	if to != domain.StatusRunning && to != domain.StatusQueued {
		t.FinishedAt = &now
	}
	b, e = json.Marshal(t)
	if e != nil {
		return false, e
	}
	res, e := tx.ExecContext(c, "UPDATE tasks SET status=?,task_json=?,finished_at=? WHERE id=? AND status=?", to, b, nullIfRunning(to, now.Unix()), id, from)
	if e != nil {
		return false, e
	}
	n, e := res.RowsAffected()
	if e != nil || n != 1 {
		return false, e
	}
	return true, tx.Commit()
}

func (s *SQLiteStore) StartTask(ctx context.Context, id string, now time.Time) (domain.Task, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Task{}, false, err
	}
	defer tx.Rollback()
	var body []byte
	if err = tx.QueryRowContext(ctx, "SELECT task_json FROM tasks WHERE id=? AND status=?", id, domain.StatusQueued).Scan(&body); errors.Is(err, sql.ErrNoRows) {
		return domain.Task{}, false, nil
	} else if err != nil {
		return domain.Task{}, false, err
	}
	var task domain.Task
	if err = json.Unmarshal(body, &task); err != nil {
		return domain.Task{}, false, err
	}
	if !domain.CanTransition(task.Status, domain.StatusRunning) {
		return domain.Task{}, false, nil
	}
	task.Status, task.Phase = domain.StatusRunning, domain.PhaseClone
	task.Attempt++
	task.ErrorCode, task.ErrorMessage = "", ""
	started := now.UTC()
	task.StartedAt, task.FinishedAt = &started, nil
	body, err = json.Marshal(task)
	if err != nil {
		return domain.Task{}, false, err
	}
	res, err := tx.ExecContext(ctx, "UPDATE tasks SET status=?,task_json=?,finished_at=NULL WHERE id=? AND status=?", task.Status, body, id, domain.StatusQueued)
	if err != nil {
		return domain.Task{}, false, err
	}
	changed, err := res.RowsAffected()
	if err != nil || changed != 1 {
		return domain.Task{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return domain.Task{}, false, err
	}
	return task, true, nil
}

func (s *SQLiteStore) SetTaskPhase(ctx context.Context, id string, attempt int, phase domain.Phase) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var body []byte
	if err = tx.QueryRowContext(ctx, "SELECT task_json FROM tasks WHERE id=? AND status=?", id, domain.StatusRunning).Scan(&body); errors.Is(err, sql.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	var task domain.Task
	if err = json.Unmarshal(body, &task); err != nil {
		return false, err
	}
	if task.Attempt != attempt {
		return false, nil
	}
	task.Phase = phase
	body, err = json.Marshal(task)
	if err != nil {
		return false, err
	}
	res, err := tx.ExecContext(ctx, "UPDATE tasks SET task_json=? WHERE id=? AND status=?", body, id, domain.StatusRunning)
	if err != nil {
		return false, err
	}
	changed, err := res.RowsAffected()
	if err != nil || changed != 1 {
		return false, err
	}
	return true, tx.Commit()
}

func (s *SQLiteStore) CompleteTask(ctx context.Context, id string, attempt int, status domain.Status, code, message string, report, logBody []byte, now time.Time) (bool, error) {
	if status != domain.StatusSucceeded && status != domain.StatusFailed && status != domain.StatusTimeout && status != domain.StatusCancelled && status != domain.StatusError {
		return false, domain.ErrInvalidTransition
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var body []byte
	if err = tx.QueryRowContext(ctx, "SELECT task_json FROM tasks WHERE id=? AND status=?", id, domain.StatusRunning).Scan(&body); errors.Is(err, sql.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	var task domain.Task
	if err = json.Unmarshal(body, &task); err != nil {
		return false, err
	}
	if task.Attempt != attempt || !domain.CanTransition(task.Status, status) {
		return false, nil
	}
	task.Status, task.ErrorCode, task.ErrorMessage = status, code, message
	finished := now.UTC()
	task.FinishedAt = &finished
	body, err = json.Marshal(task)
	if err != nil {
		return false, err
	}
	res, err := tx.ExecContext(ctx, "UPDATE tasks SET status=?,task_json=?,finished_at=? WHERE id=? AND status=?", status, body, finished.Unix(), id, domain.StatusRunning)
	if err != nil {
		return false, err
	}
	changed, err := res.RowsAffected()
	if err != nil || changed != 1 {
		return false, err
	}
	if len(report) > 0 {
		if _, err = tx.ExecContext(ctx, "INSERT INTO reports(task_id,body) VALUES(?,?) ON CONFLICT(task_id) DO UPDATE SET body=excluded.body", id, report); err != nil {
			return false, err
		}
	}
	if len(logBody) > 0 {
		if _, err = tx.ExecContext(ctx, "INSERT INTO logs(task_id,body) VALUES(?,?) ON CONFLICT(task_id) DO UPDATE SET body=excluded.body", id, logBody); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

func (s *SQLiteStore) RetryTask(ctx context.Context, id string, attempt int, code, message string, retryAt time.Time, terminal bool, now time.Time) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var body []byte
	if err = tx.QueryRowContext(ctx, "SELECT task_json FROM tasks WHERE id=? AND status=?", id, domain.StatusRunning).Scan(&body); errors.Is(err, sql.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	var task domain.Task
	if err = json.Unmarshal(body, &task); err != nil {
		return false, err
	}
	if task.Attempt != attempt {
		return false, nil
	}
	if terminal {
		task.Status = domain.StatusError
		finished := now.UTC()
		task.FinishedAt = &finished
	} else {
		task.Status, task.FinishedAt, task.StartedAt = domain.StatusQueued, nil, nil
	}
	task.ErrorCode, task.ErrorMessage = code, message
	body, err = json.Marshal(task)
	if err != nil {
		return false, err
	}
	var finishedAt any
	if task.FinishedAt != nil {
		finishedAt = task.FinishedAt.Unix()
	}
	res, err := tx.ExecContext(ctx, "UPDATE tasks SET status=?,task_json=?,finished_at=? WHERE id=? AND status=?", task.Status, body, finishedAt, id, domain.StatusRunning)
	if err != nil {
		return false, err
	}
	changed, err := res.RowsAffected()
	if err != nil || changed != 1 {
		return false, err
	}
	if !terminal {
		if _, err = tx.ExecContext(ctx, "UPDATE outbox SET available_at=?,claimed_until=0 WHERE task_id=?", retryAt.Unix(), id); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

func (s *SQLiteStore) RecoverInterrupted(ctx context.Context, now time.Time) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT id,task_json FROM tasks WHERE status=?", domain.StatusRunning)
	if err != nil {
		return 0, err
	}
	type interrupted struct {
		id   string
		body []byte
	}
	var tasks []interrupted
	for rows.Next() {
		var item interrupted
		if err = rows.Scan(&item.id, &item.body); err != nil {
			rows.Close()
			return 0, err
		}
		tasks = append(tasks, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	if err = rows.Close(); err != nil {
		return 0, err
	}
	for _, item := range tasks {
		var task domain.Task
		if err = json.Unmarshal(item.body, &task); err != nil {
			return 0, err
		}
		task.Status, task.ErrorCode, task.ErrorMessage = domain.StatusQueued, "worker_interrupted", "任务进程在服务重启时中断，将重新执行。"
		task.StartedAt, task.FinishedAt = nil, nil
		body, marshalErr := json.Marshal(task)
		if marshalErr != nil {
			return 0, marshalErr
		}
		if _, err = tx.ExecContext(ctx, "UPDATE tasks SET status=?,task_json=?,finished_at=NULL WHERE id=? AND status=?", task.Status, body, item.id, domain.StatusRunning); err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO outbox(task_id,created_at,available_at,claimed_until) VALUES(?,?,?,0) ON CONFLICT(task_id) DO UPDATE SET available_at=excluded.available_at,claimed_until=0", item.id, now.Unix(), now.Unix()); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return int64(len(tasks)), nil
}
func nullIfRunning(s domain.Status, t int64) any {
	if s == domain.StatusRunning || s == domain.StatusQueued {
		return nil
	}
	return t
}
func (s *SQLiteStore) Delete(c context.Context, id string) error {
	res, e := s.db.ExecContext(c, "DELETE FROM tasks WHERE id=?", id)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (s *SQLiteStore) DeleteExpired(c context.Context, before time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = 100
	}
	res, e := s.db.ExecContext(c, "DELETE FROM tasks WHERE id IN (SELECT id FROM tasks WHERE finished_at IS NOT NULL AND finished_at<? ORDER BY finished_at LIMIT ?)", before.Unix(), limit)
	if e != nil {
		return 0, e
	}
	return res.RowsAffected()
}
func (s *SQLiteStore) PutReport(c context.Context, id string, b []byte) error {
	_, e := s.db.ExecContext(c, "INSERT INTO reports(task_id,body) VALUES(?,?) ON CONFLICT(task_id) DO UPDATE SET body=excluded.body", id, b)
	return e
}
func (s *SQLiteStore) GetReport(c context.Context, id string) ([]byte, error) {
	var b []byte
	e := s.db.QueryRowContext(c, "SELECT body FROM reports WHERE task_id=?", id).Scan(&b)
	return b, e
}
func (s *SQLiteStore) PutLog(c context.Context, id string, b []byte) error {
	_, e := s.db.ExecContext(c, "INSERT INTO logs(task_id,body) VALUES(?,?) ON CONFLICT(task_id) DO UPDATE SET body=excluded.body", id, b)
	return e
}
func (s *SQLiteStore) GetLog(c context.Context, id string) ([]byte, error) {
	var b []byte
	e := s.db.QueryRowContext(c, "SELECT body FROM logs WHERE task_id=?", id).Scan(&b)
	return b, e
}
func (s *SQLiteStore) CreateSession(c context.Context, h []byte, expires time.Time) error {
	_, e := s.db.ExecContext(c, "INSERT INTO sessions(token_hash,expires_at) VALUES(?,?)", h, expires.Unix())
	return e
}
func (s *SQLiteStore) SessionValid(c context.Context, h []byte, now time.Time) (bool, error) {
	var n int
	e := s.db.QueryRowContext(c, "SELECT COUNT(*) FROM sessions WHERE token_hash=? AND revoked=0 AND expires_at>?", h, now.Unix()).Scan(&n)
	return n == 1, e
}
func (s *SQLiteStore) RevokeSession(c context.Context, h []byte) error {
	_, e := s.db.ExecContext(c, "UPDATE sessions SET revoked=1 WHERE token_hash=?", h)
	return e
}

func ensureTaskWorkKeyColumn(db *sql.DB) error {
	rows, err := db.Query("PRAGMA table_info(tasks)")
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, kind string
		var defaultValue any
		if err = rows.Scan(&cid, &name, &kind, &notnull, &defaultValue, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == "work_key" {
			found = true
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = db.Exec("ALTER TABLE tasks ADD COLUMN work_key TEXT")
	return err
}

type OutboxItem struct {
	ID        int64
	TaskID    string
	Attempts  int
	CreatedAt time.Time
}

func (s *SQLiteStore) CreateIdempotent(ctx context.Context, task domain.Task, identity ExecutionIdentity, now time.Time, window time.Duration) (domain.Task, bool, error) {
	key, err := identity.WorkKey()
	if err != nil {
		return domain.Task{}, false, err
	}
	repo, err := runner.ValidateRepoURL(task.RepoURL)
	identityRepo, identityErr := runner.ValidateRepoURL(identity.repoURL)
	if err != nil || identityErr != nil || repo != identityRepo || strings.ToLower(task.Commit) != strings.ToLower(identity.commitSHA) {
		return domain.Task{}, false, errors.New("task does not match execution identity")
	}
	return s.createIdempotent(ctx, task, key, now, window)
}

func (s *SQLiteStore) createIdempotent(ctx context.Context, task domain.Task, key string, now time.Time, window time.Duration) (domain.Task, bool, error) {
	if key == "" {
		return domain.Task{}, false, errors.New("empty work key")
	}
	if window <= 0 {
		window = 10 * time.Minute
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now.UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Task{}, false, err
	}
	defer tx.Rollback()
	result, reused, err := s.createIdempotentTx(ctx, tx, task, key, now, window)
	if err != nil {
		return domain.Task{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return domain.Task{}, false, err
	}
	return result, reused, nil
}

func (s *SQLiteStore) createIdempotentTx(ctx context.Context, tx *sql.Tx, task domain.Task, key string, now time.Time, window time.Duration) (domain.Task, bool, error) {
	var err error
	var body []byte
	var hasReport bool
	err = tx.QueryRowContext(ctx, "SELECT task_json, EXISTS(SELECT 1 FROM reports WHERE task_id=tasks.id) FROM tasks WHERE work_key=?", key).Scan(&body, &hasReport)
	if err == nil {
		var existing domain.Task
		if err = json.Unmarshal(body, &existing); err != nil {
			return domain.Task{}, false, err
		}
		active := existing.Status == domain.StatusQueued || existing.Status == domain.StatusRunning
		completed := existing.Status == domain.StatusSucceeded || existing.Status == domain.StatusFailed
		reusable := completed && hasReport && existing.FinishedAt != nil && now.Before(existing.FinishedAt.Add(window))
		if active || reusable {
			return existing, true, nil
		}
		if _, err = tx.ExecContext(ctx, "UPDATE tasks SET work_key=NULL WHERE work_key=?", key); err != nil {
			return domain.Task{}, false, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return domain.Task{}, false, err
	}
	body, err = json.Marshal(task)
	if err != nil {
		return domain.Task{}, false, err
	}
	var finishedAt any
	if task.FinishedAt != nil {
		finishedAt = task.FinishedAt.Unix()
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO tasks(id,status,task_json,created_at,finished_at,work_key) VALUES(?,?,?,?,?,?)", task.ID, task.Status, body, task.CreatedAt.Unix(), finishedAt, key); err != nil {
		return domain.Task{}, false, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO outbox(task_id,created_at,available_at) VALUES(?,?,?)", task.ID, now.Unix(), now.Unix()); err != nil {
		return domain.Task{}, false, err
	}
	return task, false, nil
}

func (s *SQLiteStore) StorePreview(ctx context.Context, tokenHash []byte, preview StoredPreview) error {
	if len(tokenHash) != sha256.Size || preview.ExpiresAt.IsZero() || preview.CommitSHA == "" {
		return ErrPreviewInvalid
	}
	repo, err := runner.ValidateRepoURL(preview.RepoURL)
	if err != nil || !fullSHA.MatchString(preview.CommitSHA) {
		return ErrPreviewInvalid
	}
	_, err = s.db.ExecContext(ctx, "DELETE FROM previews WHERE expires_at<=?", time.Now().Unix())
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO previews(token_hash,repo_url,ref,commit_sha,expires_at) VALUES(?,?,?,?,?)", tokenHash, repo, preview.Ref, strings.ToLower(preview.CommitSHA), preview.ExpiresAt.Unix())
	return err
}

func (s *SQLiteStore) ConfirmPreview(ctx context.Context, tokenHash []byte, task domain.Task, identity ExecutionIdentity, now time.Time, window time.Duration) (domain.Task, bool, error) {
	if len(tokenHash) != sha256.Size || task.ID == "" || task.Status != domain.StatusQueued || strings.TrimSpace(identity.goVersion) == "" || strings.TrimSpace(identity.runnerVersion) == "" {
		return domain.Task{}, false, ErrPreviewInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Task{}, false, err
	}
	defer tx.Rollback()
	var confirmedTaskID string
	var confirmedExpiry int64
	confirmErr := tx.QueryRowContext(ctx, "SELECT task_id,expires_at FROM confirmed_previews WHERE token_hash=?", tokenHash).Scan(&confirmedTaskID, &confirmedExpiry)
	if confirmErr == nil && now.Unix() < confirmedExpiry {
		var body []byte
		if err = tx.QueryRowContext(ctx, "SELECT task_json FROM tasks WHERE id=?", confirmedTaskID).Scan(&body); err != nil {
			return domain.Task{}, false, ErrPreviewInvalid
		}
		var existing domain.Task
		if err = json.Unmarshal(body, &existing); err != nil {
			return domain.Task{}, false, err
		}
		if identity.commitSHA != "" && !strings.EqualFold(existing.Commit, identity.commitSHA) || identity.repoURL != "" && existing.RepoURL != identity.repoURL {
			return domain.Task{}, false, ErrPreviewInvalid
		}
		if err = tx.Commit(); err != nil {
			return domain.Task{}, false, err
		}
		return existing, true, nil
	}
	if confirmErr != nil && !errors.Is(confirmErr, sql.ErrNoRows) {
		return domain.Task{}, false, confirmErr
	}
	if confirmErr == nil {
		if _, err = tx.ExecContext(ctx, "DELETE FROM confirmed_previews WHERE token_hash=?", tokenHash); err != nil {
			return domain.Task{}, false, err
		}
	}
	var preview StoredPreview
	var expires int64
	err = tx.QueryRowContext(ctx, "SELECT repo_url,ref,commit_sha,expires_at FROM previews WHERE token_hash=?", tokenHash).Scan(&preview.RepoURL, &preview.Ref, &preview.CommitSHA, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Task{}, false, ErrPreviewInvalid
	}
	if err != nil {
		return domain.Task{}, false, err
	}
	preview.ExpiresAt = time.Unix(expires, 0)
	if !now.Before(preview.ExpiresAt) {
		return domain.Task{}, false, ErrPreviewInvalid
	}
	identityRepo, identityErr := runner.ValidateRepoURL(identity.repoURL)
	if identityErr != nil || identityRepo != preview.RepoURL || !strings.EqualFold(identity.commitSHA, preview.CommitSHA) {
		if identity.repoURL != "" || identity.commitSHA != "" {
			return domain.Task{}, false, ErrPreviewInvalid
		}
	}
	identity.repoURL, identity.commitSHA = preview.RepoURL, preview.CommitSHA
	key, err := identity.WorkKey()
	if err != nil {
		return domain.Task{}, false, err
	}
	task.RepoURL, task.Ref, task.Commit = preview.RepoURL, preview.Ref, preview.CommitSHA
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now.UTC()
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM previews WHERE token_hash=?", tokenHash); err != nil {
		return domain.Task{}, false, err
	}
	result, reused, err := s.createIdempotentTx(ctx, tx, task, key, now, window)
	if err != nil {
		return domain.Task{}, false, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO confirmed_previews(token_hash,task_id,expires_at) VALUES(?,?,?)", tokenHash, result.ID, now.Add(10*time.Minute).Unix()); err != nil {
		return domain.Task{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return domain.Task{}, false, err
	}
	return result, reused, nil
}

func (s *SQLiteStore) ClaimOutbox(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]OutboxItem, error) {
	if lease <= 0 {
		lease = time.Minute
	}
	if limit <= 0 {
		limit = 100
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT id,task_id,attempts,created_at FROM outbox WHERE available_at<=? AND claimed_until<=? ORDER BY id LIMIT ?", now.Unix(), now.Unix(), limit)
	if err != nil {
		return nil, err
	}
	var items []OutboxItem
	for rows.Next() {
		var item OutboxItem
		var createdAt int64
		if err = rows.Scan(&item.ID, &item.TaskID, &item.Attempts, &createdAt); err != nil {
			rows.Close()
			return nil, err
		}
		item.CreatedAt = time.Unix(createdAt, 0)
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	for i := range items {
		res, e := tx.ExecContext(ctx, "UPDATE outbox SET claimed_until=?,attempts=attempts+1 WHERE id=? AND claimed_until<=?", now.Add(lease).Unix(), items[i].ID, now.Unix())
		if e != nil {
			return nil, e
		}
		n, e := res.RowsAffected()
		if e != nil {
			return nil, e
		}
		if n != 1 {
			return nil, errors.New("outbox claim lost")
		}
		items[i].Attempts++
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *SQLiteStore) AckOutbox(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM outbox WHERE id=?", id)
	return err
}
