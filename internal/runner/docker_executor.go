package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"repotest/internal/domain"
)

type Execution struct {
	ExitCode  int
	TimedOut  bool
	Output    []byte
	Truncated bool
	Report    domain.Report
}

type DockerExecutor struct {
	ImageID   string
	GoVersion string
	WorkDir   string
}

const taskWorkMarker = ".repotest-owned-work"

var goToolVersion = regexp.MustCompile(`^go version (go[0-9][^ ]*) `)

func InspectGoImage(ctx context.Context, image string) (string, error) {
	if runtime.GOOS != "linux" || !imageID.MatchString(image) {
		return "", errors.New("Go image must be a local immutable image ID on Linux")
	}
	output, err := exec.CommandContext(ctx, "docker", "run", "--rm", "--pull=never", "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=16", "--memory=128m", "--entrypoint=go", image, "version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("inspect Go runner image: %w", err)
	}
	match := goToolVersion.FindStringSubmatch(strings.TrimSpace(string(output)))
	if len(match) != 2 {
		return "", errors.New("runner image does not report a Go toolchain version")
	}
	return match[1], nil
}

func (e DockerExecutor) Execute(ctx context.Context, task domain.Task, setPhase func(domain.Phase) error) (Execution, error) {
	if runtime.GOOS != "linux" || !imageID.MatchString(e.ImageID) || e.GoVersion == "" {
		return Execution{}, errors.New("Docker executor requires a verified immutable Go image")
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Minute)
	defer cancel()
	root, err := e.ensureWorkDir()
	if err != nil {
		return Execution{}, err
	}
	parent, err := os.MkdirTemp(root, "task-")
	if err != nil {
		return Execution{}, err
	}
	defer os.RemoveAll(parent)
	if err = os.WriteFile(filepath.Join(parent, taskWorkMarker), []byte("repotest-task-work-v1\n"), 0o600); err != nil {
		return Execution{}, err
	}
	source := filepath.Join(parent, "source")
	if setPhase != nil {
		if err = setPhase(domain.PhaseClone); err != nil {
			return Execution{}, err
		}
	}
	if err = NewArchiveFetcher(nil).FetchAndExtract(ctx, task.RepoURL, task.Commit, source); err != nil {
		return Execution{}, err
	}
	result, err := runOfflineGoWithPhases(ctx, e.ImageID, source, setPhase)
	if errors.Is(err, context.DeadlineExceeded) {
		return Execution{ExitCode: result.ExitCode, TimedOut: true, Output: result.Output, Truncated: result.Truncated, Report: offlineReportWithSource(task, e.GoVersion, result, source)}, err
	}
	return Execution{ExitCode: result.ExitCode, Output: result.Output, Truncated: result.Truncated, Report: offlineReportWithSource(task, e.GoVersion, result, source)}, err
}

func (e DockerExecutor) ensureWorkDir() (string, error) {
	root := e.WorkDir
	if root == "" {
		root = filepath.Join(os.TempDir(), "repotest-work")
	}
	if !filepath.IsAbs(root) {
		return "", errors.New("runner work directory must be absolute")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("runner work directory must be a real directory")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("runner work directory must be private (0700)")
	}
	return root, nil
}

var dockerObjectID = regexp.MustCompile(`^[a-f0-9]{12,64}$`)

func (e DockerExecutor) CleanupInterrupted(ctx context.Context) error {
	containers, err := dockerObjects(ctx, "container", "ls", "-aq", "--filter", "label="+dockerManagedLabelValue())
	if err != nil {
		return err
	}
	for _, id := range containers {
		if err = exec.CommandContext(ctx, "docker", "container", "rm", "-f", id).Run(); err != nil {
			return fmt.Errorf("remove interrupted runner container: %w", err)
		}
	}
	networks, err := dockerObjects(ctx, "network", "ls", "-q", "--filter", "label="+dockerManagedLabelValue())
	if err != nil {
		return err
	}
	for _, id := range networks {
		if err = exec.CommandContext(ctx, "docker", "network", "rm", id).Run(); err != nil {
			return fmt.Errorf("remove interrupted runner network: %w", err)
		}
	}
	root, err := e.ensureWorkDir()
	if err != nil {
		return err
	}
	return cleanTaskScratch(root)
}

func cleanTaskScratch(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || len(entry.Name()) < len("task-") || entry.Name()[:len("task-")] != "task-" {
			continue
		}
		path := filepath.Join(root, entry.Name())
		info, statErr := os.Lstat(path)
		if statErr != nil {
			return statErr
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("runner scratch entry is not a real directory")
		}
		markerPath := filepath.Join(path, taskWorkMarker)
		markerInfo, markerErr := os.Lstat(markerPath)
		if errors.Is(markerErr, os.ErrNotExist) {
			continue
		}
		if markerErr != nil || !markerInfo.Mode().IsRegular() || markerInfo.Mode()&os.ModeSymlink != 0 {
			return errors.New("runner scratch ownership marker is invalid")
		}
		marker, readErr := os.ReadFile(markerPath)
		if readErr != nil || string(marker) != "repotest-task-work-v1\n" {
			return errors.New("runner scratch ownership marker does not match")
		}
		if err = os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}

func dockerObjects(ctx context.Context, args ...string) ([]string, error) {
	command := append([]string{"docker"}, args...)
	output, err := exec.CommandContext(ctx, command[0], command[1:]...).Output()
	if err != nil {
		return nil, fmt.Errorf("list interrupted runner resources: %w", err)
	}
	var ids []string
	for _, id := range strings.Fields(string(output)) {
		if !dockerObjectID.MatchString(id) {
			return nil, errors.New("Docker returned an invalid resource ID")
		}
		ids = append(ids, id)
	}
	return ids, nil
}
