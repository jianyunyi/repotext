package control

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"repotest/internal/domain"
	"repotest/internal/runner"
	"strings"
)

var configDigestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type ExecutionIdentity struct {
	repoURL       string
	commitSHA     string
	goVersion     string
	runnerVersion string
	config        []byte
}

type workIdentity struct {
	Repo          string `json:"repo"`
	Commit        string `json:"commit"`
	GoVersion     string `json:"go_version"`
	RunnerVersion string `json:"runner_version"`
	ConfigDigest  string `json:"config_digest"`
}

func ConfigurationDigest(config []byte) string {
	sum := sha256.Sum256(config)
	return hex.EncodeToString(sum[:])
}

func NewExecutionIdentity(repoURL, commitSHA, goVersion, runnerVersion string, config []byte) ExecutionIdentity {
	return ExecutionIdentity{repoURL: repoURL, commitSHA: commitSHA, goVersion: goVersion, runnerVersion: runnerVersion, config: append([]byte(nil), config...)}
}
func (e ExecutionIdentity) WorkKey() (string, error) {
	repo, err := runner.ValidateRepoURL(e.repoURL)
	if err != nil {
		return "", err
	}
	sha := strings.ToLower(e.commitSHA)
	if !fullSHA.MatchString(sha) {
		return "", errors.New("commit must be a full SHA")
	}
	if strings.TrimSpace(e.goVersion) == "" || strings.TrimSpace(e.runnerVersion) == "" {
		return "", errors.New("incomplete execution identity")
	}
	identity := workIdentity{Repo: repo, Commit: sha, GoVersion: e.goVersion, RunnerVersion: e.runnerVersion, ConfigDigest: ConfigurationDigest(e.config)}
	body, err := json.Marshal(identity)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

func (e ExecutionIdentity) isConfigured() bool {
	return strings.TrimSpace(e.goVersion) != "" && strings.TrimSpace(e.runnerVersion) != ""
}

func taskIdentityMatches(task domain.Task, current ExecutionIdentity) bool {
	if task.ExecutionIdentity == nil || !current.isConfigured() {
		return false
	}
	stored := task.ExecutionIdentity
	repoMatch := true
	if strings.TrimSpace(current.repoURL) != "" {
		repo, err := runner.ValidateRepoURL(current.repoURL)
		if err != nil {
			return false
		}
		repoMatch = stored.RepoURL == repo
	}
	commitMatch := true
	if strings.TrimSpace(current.commitSHA) != "" {
		commitMatch = strings.EqualFold(stored.CommitSHA, current.commitSHA)
	}
	return repoMatch && commitMatch && stored.GoVersion == current.goVersion && stored.RunnerVersion == current.runnerVersion && stored.ConfigDigest == ConfigurationDigest(current.config)
}
