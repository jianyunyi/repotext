package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"repotest/internal/control"
	"repotest/internal/domain"
	"repotest/internal/runner"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	dbPath := os.Getenv("REPOTEST_DB")
	if dbPath == "" {
		dbPath = "repotest.db"
	}
	store, err := control.OpenSQLiteStore(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	runnerEnabled, err := runnerEnabled(os.Getenv("REPOTEST_RUNNER_ENABLED"), os.Getenv("REPOTEST_AC008_ACCEPTED"))
	if err != nil {
		return err
	}
	var worker control.TaskWorker
	processes := control.NewTaskProcessRegistry()
	runnerImage, runnerGoVersion := "", ""
	if runnerEnabled {
		if runtime.GOOS != "linux" {
			return errors.New("runner can only be enabled on Linux")
		}
		lockPath, lockPathErr := runner.RunnerServiceLockFile()
		if lockPathErr != nil {
			return lockPathErr
		}
		workLock, lockErr := runner.AcquireRunnerWorkLock(lockPath)
		if lockErr != nil {
			return fmt.Errorf("acquire single-instance runner lock: %w", lockErr)
		}
		defer workLock.Close()
		runnerImage = strings.ToLower(strings.TrimSpace(os.Getenv("REPOTEST_GO_IMAGE")))
		goVersion, inspectErr := runner.InspectGoImage(context.Background(), runnerImage)
		if inspectErr != nil {
			return inspectErr
		}
		runnerGoVersion = goVersion
		executor := runner.DockerExecutor{ImageID: runnerImage, GoVersion: goVersion, WorkDir: os.Getenv("REPOTEST_WORK_DIR")}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 45*time.Second)
		cleanupErr := executor.CleanupInterrupted(cleanupCtx)
		cleanupCancel()
		if cleanupErr != nil {
			return fmt.Errorf("recover isolated runner resources: %w", cleanupErr)
		}
		if _, err = store.RecoverInterrupted(context.Background(), time.Now().UTC()); err != nil {
			return fmt.Errorf("recover interrupted tasks: %w", err)
		}
		worker = control.TaskWorker{Store: store, Executor: taskExecutorAdapter{executor}, Processes: processes}
	}

	pairing, code, err := control.NewPairing(store, time.Now())
	if err != nil {
		return err
	}
	api := control.API{Store: store, Processes: processes, RunnerEnabled: runnerEnabled}
	if runnerEnabled {
		api.GoVersion = runnerGoVersion
		api.RunnerVersion = runnerImage
		api.RunnerConfig = []byte("linux-offline-v1;memory=768m;cpu=1;pids=128;test-tmpfs=384m;deps-memory=768m;deps-pids=96;deps-tmpfs=640m;module-response=100m;module-total=512m")
	}
	localAddr := os.Getenv("REPOTEST_ADDR")
	if localAddr == "" {
		localAddr = "127.0.0.1:8080"
	}
	lanAddr := os.Getenv("REPOTEST_LAN_ADDR")
	localListener, lanListener, err := bindListeners(localAddr, lanAddr)
	if err != nil {
		return err
	}
	defer localListener.Close()
	if lanListener != nil {
		defer lanListener.Close()
	}

	log.Printf("LAN read-only pairing code (valid 5 minutes): %s", code)
	log.Printf("RepoTest local preview listener: %s; LAN listener configured: %t", localListener.Addr(), lanListener != nil)
	localServer := &http.Server{Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second}
	servers := []*http.Server{localServer}
	listeners := []net.Listener{localListener}
	if lanListener != nil {
		allowedHosts, err := control.LANAllowedHosts(lanListener)
		if err != nil {
			return err
		}
		handler := control.NewLANHandler(api, pairing, allowedHosts)
		servers = append(servers, &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second})
		listeners = append(listeners, lanListener)
	}
	results := make(chan error, len(servers))
	for i := range servers {
		go func(server *http.Server, listener net.Listener) { results <- server.Serve(listener) }(servers[i], listeners[i])
	}
	cleanupCtx, stopCleanup := context.WithCancel(context.Background())
	defer stopCleanup()
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-cleanupCtx.Done():
				return
			case <-ticker.C:
				if _, err := store.DeleteExpired(cleanupCtx, time.Now().Add(-7*24*time.Hour), 100); err != nil {
					log.Printf("TTL cleanup error: %v", err)
				}
			}
		}
	}()

	serveCtx, stopServing := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopServing()
	workerCtx, stopWorker := context.WithCancel(context.Background())
	defer stopWorker()
	workerDone := make(chan error, 1)
	if runnerEnabled {
		go func() { workerDone <- worker.Run(workerCtx) }()
	}
	var firstErr error
	select {
	case firstErr = <-results:
	case <-serveCtx.Done():
		firstErr = serveCtx.Err()
	}
	stopWorker()
	stopCleanup()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	for _, server := range servers {
		_ = server.Shutdown(shutdownCtx)
	}
	shutdownCancel()
	if runnerEnabled {
		select {
		case workerErr := <-workerDone:
			if workerErr != nil && !errors.Is(workerErr, context.Canceled) {
				log.Printf("runner worker stopped: %v", workerErr)
			}
		case <-time.After(20 * time.Second):
			log.Printf("runner worker did not stop within shutdown grace period")
		}
	}
	if errors.Is(firstErr, context.Canceled) || errors.Is(firstErr, http.ErrServerClosed) {
		return nil
	}
	return firstErr
}

func runnerEnabled(enabled, accepted string) (bool, error) {
	if enabled != "1" {
		return false, nil
	}
	if accepted != "1" {
		return false, errors.New("execution requires explicit AC-008 acceptance after target Linux verification")
	}
	return true, nil
}

type taskExecutorAdapter struct{ executor runner.DockerExecutor }

func (a taskExecutorAdapter) Execute(ctx context.Context, task domain.Task, setPhase func(domain.Phase) error) (control.TaskExecution, error) {
	result, err := a.executor.Execute(ctx, task, setPhase)
	return control.TaskExecution{ExitCode: result.ExitCode, TimedOut: result.TimedOut, Output: result.Output, Truncated: result.Truncated, Report: result.Report}, err
}

func bindListeners(localAddr, lanAddr string) (net.Listener, net.Listener, error) {
	host, _, err := net.SplitHostPort(localAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid loopback address: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return nil, nil, fmt.Errorf("local listener must bind to an explicit loopback IP")
	}
	local, err := net.Listen("tcp", localAddr)
	if err != nil {
		return nil, nil, err
	}
	if lanAddr == "" {
		return local, nil, nil
	}
	lan, err := net.Listen("tcp", lanAddr)
	if err != nil {
		_ = local.Close()
		return nil, nil, err
	}
	return local, lan, nil
}
