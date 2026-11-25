package control

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveRefDereferencesAnnotatedTag(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: previewRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		switch {
		case strings.HasSuffix(r.URL.Path, "/git/ref/tags/v2.0.0"):
			return jsonResponse(200, `{"object":{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","type":"tag"}}`), nil
		case strings.HasSuffix(r.URL.Path, "/git/tags/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"):
			return jsonResponse(200, `{"object":{"sha":"0123456789012345678901234567890123456789","type":"commit"}}`), nil
		}
		t.Fatalf("unexpected URL %s", r.URL)
		return nil, nil
	})}
	got, err := resolveRef(context.Background(), client, "acme", "widget", "tags", "v2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got != "0123456789012345678901234567890123456789" || calls != 2 {
		t.Fatalf("sha=%s calls=%d", got, calls)
	}
}
func TestResolveRefRejectsNestedTagBeyondLimit(t *testing.T) {
	client := &http.Client{Transport: previewRoundTripper(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(200, `{"object":{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","type":"tag"}}`), nil
	})}
	if _, err := resolveRef(context.Background(), client, "acme", "widget", "tags", "loop"); err == nil {
		t.Fatal("expected tag recursion limit to reject unresolved tag")
	}
}

var _ = httptest.NewRequest
