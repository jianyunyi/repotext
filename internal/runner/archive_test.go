package runner

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const archiveTestSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type archiveRoundTripper func(*http.Request) (*http.Response, error)

func (f archiveRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestArchiveFetcherFetchesPinnedCommitAndExtractsFiles(t *testing.T) {
	wantArchive := makeTarGz(t, []tarFixture{{name: "repo-root/go.mod", kind: tar.TypeReg, body: "module sample\n"}})
	var apiRequests, archiveRequests int
	client := &http.Client{Transport: archiveRoundTripper(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.URL.Host == "api.github.com":
			apiRequests++
			if r.URL.Path != "/repos/acme/widget/tarball/"+archiveTestSHA {
				t.Fatalf("unexpected archive endpoint %s", r.URL)
			}
			return archiveResponse(r, http.StatusFound, "https://codeload.github.com/acme/widget/legacy.tar.gz/"+archiveTestSHA, nil), nil
		case r.URL.Host == "codeload.github.com":
			archiveRequests++
			if r.URL.Scheme != "https" {
				t.Fatalf("archive request downgraded: %s", r.URL)
			}
			return archiveResponse(r, http.StatusOK, "", wantArchive), nil
		default:
			return nil, fmt.Errorf("unexpected request host %q", r.URL.Host)
		}
	})}
	dest := newArchiveTestDestination(t)
	fetcher := newTestArchiveFetcher(client)
	if err := fetcher.FetchAndExtract(context.Background(), "https://github.com/acme/widget", archiveTestSHA, dest); err != nil {
		t.Fatal(err)
	}
	if apiRequests != 1 || archiveRequests != 1 {
		t.Fatalf("requests: api=%d archive=%d", apiRequests, archiveRequests)
	}
	got, err := os.ReadFile(filepath.Join(dest, "go.mod"))
	if err != nil || string(got) != "module sample\n" {
		t.Fatalf("extracted go.mod=%q err=%v", got, err)
	}
}

func TestArchiveFetcherAcceptsGitHubPAXGlobalComment(t *testing.T) {
	wantArchive := makePAXGlobalTarGz(t, map[string]string{"comment": "generated"}, []tarFixture{
		{name: "repo-root/go.mod", kind: tar.TypeReg, body: "module sample\n"},
	})
	client := &http.Client{Transport: archiveRoundTripper(func(r *http.Request) (*http.Response, error) {
		request := r.Clone(r.Context())
		request.URL, _ = url.Parse("https://codeload.github.com/acme/widget/archive.tar.gz/" + archiveTestSHA)
		return archiveResponse(request, http.StatusOK, "", wantArchive), nil
	})}
	dest := newArchiveTestDestination(t)
	if err := newTestArchiveFetcher(client).FetchAndExtract(context.Background(), "https://github.com/acme/widget", archiveTestSHA, dest); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dest, "go.mod")); err != nil || string(got) != "module sample\n" {
		t.Fatalf("extracted go.mod=%q err=%v", got, err)
	}
}

func TestArchiveFetcherRejectsPAXGlobalPathMetadata(t *testing.T) {
	archive := makePAXGlobalTarGz(t, map[string]string{"path": "../../escape"}, []tarFixture{
		{name: "repo-root/go.mod", kind: tar.TypeReg, body: "module sample\n"},
	})
	client := &http.Client{Transport: archiveRoundTripper(func(r *http.Request) (*http.Response, error) {
		request := r.Clone(r.Context())
		request.URL, _ = url.Parse("https://codeload.github.com/acme/widget/archive.tar.gz/" + archiveTestSHA)
		return archiveResponse(request, http.StatusOK, "", archive), nil
	})}
	if err := newTestArchiveFetcher(client).FetchAndExtract(context.Background(), "https://github.com/acme/widget", archiveTestSHA, newArchiveTestDestination(t)); err == nil {
		t.Fatal("accepted unsafe global PAX path metadata")
	}
}

func makePAXGlobalTarGz(t *testing.T, records map[string]string, entries []tarFixture) []byte {
	t.Helper()
	var out bytes.Buffer
	zipper := gzip.NewWriter(&out)
	w := tar.NewWriter(zipper)
	if err := w.WriteHeader(&tar.Header{Name: "pax_global_header", Typeflag: tar.TypeXGlobalHeader, PAXRecords: records}); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		mode := int64(0644)
		header := &tar.Header{Name: entry.name, Typeflag: entry.kind, Mode: mode, Linkname: entry.link, Size: int64(len(entry.body)), Format: entry.format}
		if entry.kind == tar.TypeDir {
			header.Size = 0
		}
		if err := w.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if entry.kind == tar.TypeReg && entry.body != "" {
			if _, err := io.Copy(w, strings.NewReader(entry.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zipper.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestArchiveFetcherRejectsInvalidInputsWithoutNetwork(t *testing.T) {
	requests := 0
	fetcher := newTestArchiveFetcher(&http.Client{Transport: archiveRoundTripper(func(*http.Request) (*http.Response, error) {
		requests++
		return nil, fmt.Errorf("request should not be made")
	})})
	for _, tc := range []struct{ repo, sha string }{
		{"https://example.com/acme/widget", archiveTestSHA},
		{"https://github.com/acme/widget", "abc"},
	} {
		if err := fetcher.FetchAndExtract(context.Background(), tc.repo, tc.sha, newArchiveTestDestination(t)); err == nil {
			t.Fatalf("accepted repo=%q sha=%q", tc.repo, tc.sha)
		}
	}
	if requests != 0 {
		t.Fatalf("invalid inputs made %d network requests", requests)
	}
}

func TestArchiveFetcherRequiresPrivateTaskParentBeforeNetwork(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("POSIX directory permission check is Linux-specific")
	}
	parent := t.TempDir()
	if err := os.Chmod(parent, 0755); err != nil {
		t.Fatal(err)
	}
	requests := 0
	fetcher := newTestArchiveFetcher(&http.Client{Transport: archiveRoundTripper(func(*http.Request) (*http.Response, error) {
		requests++
		return nil, fmt.Errorf("network access forbidden in this test")
	})})
	if err := fetcher.FetchAndExtract(context.Background(), "https://github.com/acme/widget", archiveTestSHA, filepath.Join(parent, "workspace")); err == nil {
		t.Fatal("non-private task parent accepted")
	}
	if requests != 0 {
		t.Fatalf("non-private parent caused %d requests", requests)
	}
}

func TestArchiveFetcherRejectsUnsafeRedirects(t *testing.T) {
	for _, target := range []string{
		"http://codeload.github.com/acme/widget/archive.tar.gz",
		"https://127.0.0.1/archive.tar.gz",
		"https://user@codeload.github.com/archive.tar.gz",
		"https://codeload.github.com:444/archive.tar.gz",
		"https://attacker.example/archive.tar.gz",
	} {
		t.Run(target, func(t *testing.T) {
			archiveRequests := 0
			client := &http.Client{Transport: archiveRoundTripper(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "api.github.com" {
					archiveRequests++
					return archiveResponse(r, http.StatusOK, "", []byte("unused")), nil
				}
				return archiveResponse(r, http.StatusFound, target, nil), nil
			})}
			fetcher := newTestArchiveFetcher(client)
			if err := fetcher.FetchAndExtract(context.Background(), "https://github.com/acme/widget", archiveTestSHA, newArchiveTestDestination(t)); err == nil {
				t.Fatal("unsafe redirect accepted")
			}
			if archiveRequests != 0 {
				t.Fatalf("unsafe redirect reached archive transport %d times", archiveRequests)
			}
		})
	}
}

func TestArchiveFetcherRejectsSecondRedirectAndBadStatus(t *testing.T) {
	t.Run("second redirect", func(t *testing.T) {
		client := &http.Client{Transport: archiveRoundTripper(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host == "api.github.com" {
				return archiveResponse(r, http.StatusFound, "https://codeload.github.com/first", nil), nil
			}
			return archiveResponse(r, http.StatusFound, "https://codeload.github.com/second", nil), nil
		})}
		if err := newTestArchiveFetcher(client).FetchAndExtract(context.Background(), "https://github.com/acme/widget", archiveTestSHA, newArchiveTestDestination(t)); err == nil {
			t.Fatal("second redirect accepted")
		}
	})
	t.Run("archive status", func(t *testing.T) {
		client := &http.Client{Transport: archiveRoundTripper(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host == "api.github.com" {
				return archiveResponse(r, http.StatusFound, "https://codeload.github.com/archive", nil), nil
			}
			return archiveResponse(r, http.StatusForbidden, "", []byte("denied")), nil
		})}
		if err := newTestArchiveFetcher(client).FetchAndExtract(context.Background(), "https://github.com/acme/widget", archiveTestSHA, newArchiveTestDestination(t)); err == nil {
			t.Fatal("non-200 archive response accepted")
		}
	})
}

func TestArchiveFetcherRejectsWrongRepositoryRedirectAndCleansFailedWorkspace(t *testing.T) {
	t.Run("wrong archive path", func(t *testing.T) {
		archiveRequests := 0
		client := &http.Client{Transport: archiveRoundTripper(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host == "api.github.com" {
				return archiveResponse(r, http.StatusFound, "https://codeload.github.com/other/repo/archive.tar.gz/"+archiveTestSHA, nil), nil
			}
			archiveRequests++
			return archiveResponse(r, http.StatusOK, "", []byte("unused")), nil
		})}
		if err := newTestArchiveFetcher(client).FetchAndExtract(context.Background(), "https://github.com/acme/widget", archiveTestSHA, newArchiveTestDestination(t)); err == nil {
			t.Fatal("wrong repository redirect accepted")
		}
		if archiveRequests != 0 {
			t.Fatalf("wrong archive path reached transport %d times", archiveRequests)
		}
	})
	t.Run("invalid archive removes workspace", func(t *testing.T) {
		client := &http.Client{Transport: archiveRoundTripper(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host == "api.github.com" {
				return archiveResponse(r, http.StatusFound, "https://codeload.github.com/acme/widget/legacy.tar.gz/"+archiveTestSHA, nil), nil
			}
			return archiveResponse(r, http.StatusOK, "", []byte("not a tarball")), nil
		})}
		dest := newArchiveTestDestination(t)
		if err := newTestArchiveFetcher(client).FetchAndExtract(context.Background(), "https://github.com/acme/widget", archiveTestSHA, dest); err == nil {
			t.Fatal("invalid archive accepted")
		}
		if _, err := os.Lstat(dest); !os.IsNotExist(err) {
			t.Fatalf("failed destination was not removed: %v", err)
		}
	})
}

func TestArchiveFetcherNeverForwardsHTTPJarCookies(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"https://api.github.com/repos/acme/widget/tarball/" + archiveTestSHA, "https://codeload.github.com/acme/widget/legacy.tar.gz/" + archiveTestSHA} {
		u, err := http.NewRequest(http.MethodGet, raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		jar.SetCookies(u.URL, []*http.Cookie{{Name: "session", Value: "sensitive"}})
	}
	archive := makeTarGz(t, []tarFixture{{name: "root/file", kind: tar.TypeReg, body: "x"}})
	client := &http.Client{Jar: jar, Transport: archiveRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Cookie") != "" {
			return nil, fmt.Errorf("cookie leaked to %s", r.URL.Host)
		}
		if r.URL.Host == "api.github.com" {
			return archiveResponse(r, http.StatusFound, "https://codeload.github.com/acme/widget/archive.tar.gz/"+archiveTestSHA, nil), nil
		}
		return archiveResponse(r, http.StatusOK, "", archive), nil
	})}
	if err := newTestArchiveFetcher(client).FetchAndExtract(context.Background(), "https://github.com/acme/widget", archiveTestSHA, newArchiveTestDestination(t)); err != nil {
		t.Fatal(err)
	}
}

func newArchiveTestDestination(t *testing.T) string {
	t.Helper()
	parent, err := os.MkdirTemp(t.TempDir(), "private")
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(parent, "workspace")
}

func TestArchiveExtractRejectsUnsafeEntryTypesAndPaths(t *testing.T) {
	cases := []struct {
		name    string
		entries []tarFixture
	}{
		{"parent traversal", []tarFixture{{name: "repo-root/../../escape", kind: tar.TypeReg, body: "x"}}},
		{"absolute path", []tarFixture{{name: "/repo-root/file", kind: tar.TypeReg, body: "x"}}},
		{"backslash", []tarFixture{{name: `repo-root\..\escape`, kind: tar.TypeReg, body: "x"}}},
		{"symlink", []tarFixture{{name: "repo-root/link", kind: tar.TypeSymlink, link: "../../escape"}}},
		{"hardlink", []tarFixture{{name: "repo-root/link", kind: tar.TypeLink, link: "repo-root/file"}}},
		{"fifo", []tarFixture{{name: "repo-root/pipe", kind: tar.TypeFifo}}},
		{"duplicate file", []tarFixture{{name: "repo-root/file", kind: tar.TypeReg, body: "a"}, {name: "repo-root/file", kind: tar.TypeReg, body: "b"}}},
		{"multiple roots", []tarFixture{{name: "first/file", kind: tar.TypeReg, body: "a"}, {name: "second/file", kind: tar.TypeReg, body: "b"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "workspace")
			if err := os.Mkdir(dest, 0700); err != nil {
				t.Fatal(err)
			}
			if err := extractTarGz(bytes.NewReader(makeTarGz(t, tc.entries)), dest, archiveLimits{maxCompressed: 1 << 20, maxExpanded: 1 << 20, maxFileBytes: 1 << 20, maxEntries: 20}); err == nil {
				t.Fatal("unsafe archive accepted")
			}
			entries, err := os.ReadDir(dest)
			if err != nil || len(entries) != 0 {
				t.Fatalf("destination not cleaned: entries=%v err=%v", entries, err)
			}
		})
	}
}

func TestArchiveExtractEnforcesAllSizeAndEntryLimits(t *testing.T) {
	cases := []struct {
		name   string
		limits archiveLimits
		files  []tarFixture
	}{
		{"compressed bytes", archiveLimits{maxCompressed: 30, maxExpanded: 1 << 20, maxFileBytes: 1 << 20, maxEntries: 20}, []tarFixture{{name: "repo/file", kind: tar.TypeReg, body: strings.Repeat("x", 128)}}},
		{"expanded bytes", archiveLimits{maxCompressed: 1 << 20, maxExpanded: 128, maxFileBytes: 1 << 20, maxEntries: 20}, []tarFixture{{name: "repo/file", kind: tar.TypeReg, body: strings.Repeat("x", 256)}}},
		{"file payload", archiveLimits{maxCompressed: 1 << 20, maxExpanded: 1 << 20, maxFileBytes: 10, maxEntries: 20}, []tarFixture{{name: "repo/file", kind: tar.TypeReg, body: strings.Repeat("x", 11)}}},
		{"entries", archiveLimits{maxCompressed: 1 << 20, maxExpanded: 1 << 20, maxFileBytes: 1 << 20, maxEntries: 1}, []tarFixture{{name: "repo/a", kind: tar.TypeReg, body: "a"}, {name: "repo/b", kind: tar.TypeReg, body: "b"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "workspace")
			if err := os.Mkdir(dest, 0700); err != nil {
				t.Fatal(err)
			}
			archive := makeTarGz(t, tc.files)
			if err := extractTarGz(bytes.NewReader(archive), dest, tc.limits); err == nil {
				t.Fatal("over-limit archive accepted")
			}
			entries, err := os.ReadDir(dest)
			if err != nil || len(entries) != 0 {
				t.Fatalf("destination not cleaned: entries=%v err=%v", entries, err)
			}
		})
	}
}

func TestArchiveExtractRequiresEmptyDestination(t *testing.T) {
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(dest, "existing"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := extractTarGz(bytes.NewReader(makeTarGz(t, []tarFixture{{name: "root/file", kind: tar.TypeReg, body: "x"}})), dest, archiveLimits{maxCompressed: 1 << 20, maxExpanded: 1 << 20, maxFileBytes: 1 << 20, maxEntries: 20}); err == nil {
		t.Fatal("non-empty destination accepted")
	}
	got, err := os.ReadFile(filepath.Join(dest, "existing"))
	if err != nil || string(got) != "keep" {
		t.Fatalf("existing content changed: %q err=%v", got, err)
	}
}

func TestArchiveExtractRejectsCorruptGzipTrailerAndCleansFiles(t *testing.T) {
	archive := makeTarGz(t, []tarFixture{{name: "root/file", kind: tar.TypeReg, body: "written before bad trailer"}})
	archive[len(archive)-1] ^= 0xff
	dest := filepath.Join(t.TempDir(), "workspace")
	if err := os.Mkdir(dest, 0700); err != nil {
		t.Fatal(err)
	}
	if err := extractTarGz(bytes.NewReader(archive), dest, archiveLimits{maxCompressed: 1 << 20, maxExpanded: 1 << 20, maxFileBytes: 1 << 20, maxEntries: 20}); err == nil {
		t.Fatal("corrupt gzip trailer accepted")
	}
	entries, err := os.ReadDir(dest)
	if err != nil || len(entries) != 0 {
		t.Fatalf("corrupt archive left files: entries=%v err=%v", entries, err)
	}
}

func TestArchiveExtractValidatesPAXPathsAndSiblingNames(t *testing.T) {
	t.Run("pax traversal", func(t *testing.T) {
		name := "root/" + strings.Repeat("x", 120) + "/../../escape"
		dest := filepath.Join(t.TempDir(), "workspace")
		if err := os.Mkdir(dest, 0700); err != nil {
			t.Fatal(err)
		}
		archive := makeTarGz(t, []tarFixture{{name: name, kind: tar.TypeReg, body: "bad", format: tar.FormatPAX}})
		if err := extractTarGz(bytes.NewReader(archive), dest, archiveLimits{maxCompressed: 1 << 20, maxExpanded: 1 << 20, maxFileBytes: 1 << 20, maxEntries: 20}); err == nil {
			t.Fatal("PAX traversal path accepted")
		}
	})
	t.Run("sibling names", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "workspace")
		if err := os.Mkdir(dest, 0700); err != nil {
			t.Fatal(err)
		}
		archive := makeTarGz(t, []tarFixture{{name: "root/a", kind: tar.TypeReg, body: "a"}, {name: "root/a-b", kind: tar.TypeReg, body: "b"}})
		if err := extractTarGz(bytes.NewReader(archive), dest, archiveLimits{maxCompressed: 1 << 20, maxExpanded: 1 << 20, maxFileBytes: 1 << 20, maxEntries: 20}); err != nil {
			t.Fatal(err)
		}
		for name, value := range map[string]string{"a": "a", "a-b": "b"} {
			got, err := os.ReadFile(filepath.Join(dest, name))
			if err != nil || string(got) != value {
				t.Fatalf("%s=%q err=%v", name, got, err)
			}
		}
	})
}

type tarFixture struct {
	name   string
	kind   byte
	body   string
	link   string
	format tar.Format
}

func makeTarGz(t *testing.T, entries []tarFixture) []byte {
	t.Helper()
	var out bytes.Buffer
	zipper := gzip.NewWriter(&out)
	w := tar.NewWriter(zipper)
	for _, entry := range entries {
		mode := int64(0644)
		header := &tar.Header{Name: entry.name, Typeflag: entry.kind, Mode: mode, Linkname: entry.link, Size: int64(len(entry.body)), Format: entry.format}
		if entry.kind == tar.TypeDir {
			header.Size = 0
		}
		if err := w.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if entry.kind == tar.TypeReg && entry.body != "" {
			if _, err := io.Copy(w, strings.NewReader(entry.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zipper.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func archiveResponse(req *http.Request, status int, location string, body []byte) *http.Response {
	resp := &http.Response{StatusCode: status, ContentLength: int64(len(body)), Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body)), Request: req}
	if location != "" {
		resp.Header.Set("Location", location)
	}
	return resp
}

func newTestArchiveFetcher(client *http.Client) ArchiveFetcher {
	return ArchiveFetcher{client: client, apiBase: "https://api.github.com", archiveHost: "codeload.github.com", limits: archiveLimits{maxCompressed: 1 << 20, maxExpanded: 1 << 20, maxFileBytes: 1 << 20, maxEntries: 20}}
}
