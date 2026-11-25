package control

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestWriteErrorReturnsStableJSONEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, 400, "invalid_url")
	if rec.Code != 400 {
		t.Fatalf("status=%d", rec.Code)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "invalid_url" {
		t.Fatalf("body=%s", rec.Body.String())
	}
}
