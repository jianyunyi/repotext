package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"
)

type ReadSession struct {
	ID        string    `json:"id"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *SQLiteStore) ListReadSessions(ctx context.Context, now time.Time) ([]ReadSession, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT token_hash,expires_at FROM sessions WHERE revoked=0 AND expires_at>? ORDER BY expires_at DESC,token_hash", now.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions := make([]ReadSession, 0)
	for rows.Next() {
		var hash []byte
		var expires int64
		if err := rows.Scan(&hash, &expires); err != nil {
			return nil, err
		}
		sessions = append(sessions, ReadSession{ID: hex.EncodeToString(hash), ExpiresAt: time.Unix(expires, 0).UTC()})
	}
	return sessions, rows.Err()
}

func (a API) listSessions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	store, ok := a.Store.(*SQLiteStore)
	if !ok || store == nil {
		writeError(w, http.StatusServiceUnavailable, "storage_unavailable")
		return
	}
	sessions, err := store.ListReadSessions(r.Context(), time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (a API) revokeSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	store, ok := a.Store.(*SQLiteStore)
	if !ok || store == nil {
		writeError(w, http.StatusServiceUnavailable, "storage_unavailable")
		return
	}
	hash, err := hex.DecodeString(r.PathValue("id"))
	if err != nil || len(hash) != sha256.Size {
		writeError(w, http.StatusBadRequest, "invalid_session")
		return
	}
	if err := store.RevokeSession(r.Context(), hash); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
