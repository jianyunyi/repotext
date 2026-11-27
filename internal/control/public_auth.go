package control

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"repotest/internal/domain"
)

type CaptchaVerifier interface {
	Verify(context.Context, string, string) error
}

var (
	ErrCaptchaUnavailable = errors.New("captcha verifier unavailable")
	ErrCaptchaInvalid     = errors.New("captcha verification failed")
	ErrPublicUnauthorized = errors.New("public task authorization failed")
	ErrCleanupPending     = errors.New("task cleanup pending")
)

type CaptchaVerifierFunc func(context.Context, string, string) error

func (f CaptchaVerifierFunc) Verify(ctx context.Context, token, remoteAddr string) error {
	return f(ctx, token, remoteAddr)
}

func (a API) verifyCaptcha(ctx context.Context, r *http.Request, token string) error {
	if a.Captcha == nil {
		return ErrCaptchaUnavailable
	}
	if strings.TrimSpace(token) == "" {
		return ErrCaptchaInvalid
	}
	remoteAddr := r.RemoteAddr
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		remoteAddr = host
	}
	if err := a.Captcha.Verify(ctx, token, remoteAddr); err != nil {
		if errors.Is(err, ErrCaptchaUnavailable) {
			return err
		}
		return errors.Join(ErrCaptchaInvalid, err)
	}
	return nil
}

func newControlToken() (string, []byte, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", nil, err
	}
	hash := sha256.Sum256(value)
	return base64.RawURLEncoding.EncodeToString(value), hash[:], nil
}

func (a API) publicPreview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		previewRequest
		CaptchaToken string `json:"captcha_token"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req) != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := a.verifyCaptcha(r.Context(), r, req.CaptchaToken); err != nil {
		writePublicWriteError(w, err)
		return
	}
	body, err := json.Marshal(req.previewRequest)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	a.preview(w, r)
}

func (a API) publicConfirm(w http.ResponseWriter, r *http.Request) {
	if !a.RunnerEnabled {
		writeError(w, http.StatusServiceUnavailable, "runner_disabled")
		return
	}
	if strings.TrimSpace(a.GoVersion) == "" || strings.TrimSpace(a.RunnerVersion) == "" {
		writeError(w, http.StatusServiceUnavailable, "runner_not_configured")
		return
	}
	var req confirmRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	previewToken, err := base64.RawURLEncoding.DecodeString(req.PreviewID)
	if err != nil || len(previewToken) != 32 {
		writeError(w, http.StatusBadRequest, "invalid_preview")
		return
	}
	store, ok := a.Store.(publicPreviewConfirmationStore)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "storage_unavailable")
		return
	}
	controlToken, controlHash, err := newControlToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	var idBytes [16]byte
	if _, err = rand.Read(idBytes[:]); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	now := time.Now().UTC()
	task := domain.Task{ID: hexEncode(idBytes[:]), Status: domain.StatusQueued, CreatedAt: now}
	identity := ExecutionIdentity{goVersion: a.GoVersion, runnerVersion: a.RunnerVersion, config: a.RunnerConfig}
	previewHash := sha256.Sum256(previewToken)
	created, reused, err := store.ConfirmPreviewWithToken(r.Context(), previewHash[:], task, identity, controlHash, now, 10*time.Minute)
	if err != nil {
		writeTaskMutationError(w, err)
		return
	}
	response := map[string]any{"task": publicTaskProjection(created), "reused": reused}
	if !reused {
		response["control_token"] = controlToken
	}
	status := http.StatusAccepted
	if reused {
		status = http.StatusOK
	}
	writeJSON(w, status, response)
}

func hexEncode(value []byte) string {
	const alphabet = "0123456789abcdef"
	result := make([]byte, len(value)*2)
	for index, item := range value {
		result[index*2] = alphabet[item>>4]
		result[index*2+1] = alphabet[item&15]
	}
	return string(result)
}

func (a API) authorizePublicTask(ctx context.Context, r *http.Request) error {
	if !a.publicOriginAllowed(r) {
		return ErrPublicUnauthorized
	}
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return ErrPublicUnauthorized
	}
	encoded := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	value, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(value) != 32 {
		return ErrPublicUnauthorized
	}
	hash := sha256.Sum256(value)
	store, ok := a.Store.(TaskTokenStore)
	if !ok {
		return ErrPublicUnauthorized
	}
	stored, err := store.GetTaskTokenHash(ctx, r.PathValue("id"))
	if err != nil || len(stored) != len(hash) || subtle.ConstantTimeCompare(stored, hash[:]) != 1 {
		return ErrPublicUnauthorized
	}
	return nil
}

func (a API) publicCancel(w http.ResponseWriter, r *http.Request) {
	if err := a.authorizePublicTask(r.Context(), r); err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := a.cancelTask(r.Context(), r.PathValue("id")); err != nil {
		writePublicTaskMutationError(w, err)
		return
	}
	task, err := a.Store.Get(r.Context(), r.PathValue("id"))
	if err != nil || task.Status != domain.StatusCancelled {
		if err == nil {
			err = errors.New("task not cancelled")
		}
		writePublicTaskMutationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a API) publicDelete(w http.ResponseWriter, r *http.Request) {
	if err := a.authorizePublicTask(r.Context(), r); err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := r.PathValue("id")
	task, err := a.Store.Get(r.Context(), id)
	if err != nil {
		writePublicTaskMutationError(w, err)
		return
	}
	if task.Status == domain.StatusQueued || task.Status == domain.StatusRunning {
		if err = a.cancelTask(r.Context(), id); err != nil {
			writePublicTaskMutationError(w, err)
			return
		}
		task, err = a.Store.Get(r.Context(), id)
		if err != nil || task.Status == domain.StatusQueued || task.Status == domain.StatusRunning {
			writePublicTaskMutationError(w, errors.New("task still active"))
			return
		}
	}
	if err = a.Store.Delete(r.Context(), id); err != nil {
		writePublicTaskMutationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writePublicWriteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrCaptchaUnavailable):
		writeError(w, http.StatusServiceUnavailable, "captcha_unavailable")
	case errors.Is(err, ErrCaptchaInvalid):
		writeError(w, http.StatusForbidden, "captcha_failed")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error")
	}
}

func writePublicTaskMutationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "task_not_found")
	case errors.Is(err, ErrCleanupPending):
		writeError(w, http.StatusConflict, "cleanup_pending")
	case errors.Is(err, ErrTaskProcessNotRegistered), errors.Is(err, domain.ErrInvalidTransition):
		writeError(w, http.StatusConflict, "task_not_cancellable")
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "process_reap_timeout")
	default:
		writeError(w, http.StatusConflict, "task_mutation_rejected")
	}
}

func (a API) publicOriginAllowed(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" || len(a.PublicOrigins) == 0 {
		return true
	}
	for _, allowed := range a.PublicOrigins {
		if origin == strings.TrimSpace(allowed) {
			return true
		}
	}
	return false
}

func publicTaskProjection(task domain.Task) publicTask {
	return publicTask{ID: task.ID, RepoURL: task.RepoURL, Ref: task.Ref, Commit: task.Commit, Status: task.Status, Phase: task.Phase, CreatedAt: task.CreatedAt, StartedAt: task.StartedAt, FinishedAt: task.FinishedAt}
}
