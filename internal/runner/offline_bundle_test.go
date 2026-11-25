package runner

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceBundleSeparatesSourceAndModuleCache(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "go.mod"), []byte("module example.test/app\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	modules := makeTar(t, []tarEntry{{name: "cache/download/example.test/lib/@v/v1.0.0.mod", body: "module example.test/lib\n"}})
	var bundle bytes.Buffer
	if err := writeWorkspaceBundle(&bundle, source, bytes.NewReader(modules)); err != nil {
		t.Fatal(err)
	}
	entries := readTarEntries(t, bundle.Bytes())
	if got := entries["src/go.mod"]; got != "module example.test/app\n" {
		t.Fatalf("source entry=%q", got)
	}
	if got := entries["modules/cache/download/example.test/lib/@v/v1.0.0.mod"]; got != "module example.test/lib\n" {
		t.Fatalf("module cache entry=%q", got)
	}
}

func TestWorkspaceBundleAcceptsEmptyModuleCache(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "go.mod"), []byte("module example.test/app\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var bundle bytes.Buffer
	if err := writeWorkspaceBundle(&bundle, source, bytes.NewReader(nil)); err != nil {
		t.Fatal(err)
	}
	entries := readTarEntries(t, bundle.Bytes())
	if _, ok := entries["src/go.mod"]; !ok {
		t.Fatal("source missing from bundle")
	}
}

func TestWorkspaceBundleNormalizesDockerCopyRootHeaders(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "go.mod"), []byte("module example.test/app\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	modules := makeTar(t, []tarEntry{
		{name: "./", typeflag: tar.TypeDir},
		{name: "modules/", typeflag: tar.TypeDir},
		{name: "modules/cache/download/example.test/lib/@v/v1.0.0.mod", body: "module example.test/lib\n"},
	})
	var bundle bytes.Buffer
	if err := writeWorkspaceBundle(&bundle, source, bytes.NewReader(modules)); err != nil {
		t.Fatal(err)
	}
	entries := readTarEntries(t, bundle.Bytes())
	if entries["modules/cache/download/example.test/lib/@v/v1.0.0.mod"] != "module example.test/lib\n" {
		t.Fatalf("module cache root was not normalized: %+v", entries)
	}
}

func TestWorkspaceBundleRejectsUnsafeModuleCacheEntries(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "go.mod"), []byte("module example.test/app\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []tarEntry{
		{name: "../escape", body: "x"},
		{name: "/absolute", body: "x"},
		{name: `bad\path`, body: "x"},
		{name: "link", typeflag: tar.TypeSymlink, linkname: "../../host"},
	} {
		t.Run(entry.name, func(t *testing.T) {
			archive := makeTar(t, []tarEntry{entry})
			var bundle bytes.Buffer
			if err := writeWorkspaceBundle(&bundle, source, bytes.NewReader(archive)); err == nil {
				t.Fatal("unsafe module cache entry accepted")
			}
		})
	}
}

func TestDependencyContainerUsesInternalNetworkAndFixedProxy(t *testing.T) {
	args := dependencyContainerArgs("repotest-deps-test", "http://172.30.0.1:45123")
	joined := strings.Join(args, "\n")
	for _, required := range []string{"-i", "--network=repotest-deps-test", "--read-only", "--cap-drop=ALL", "--pids-limit=96", "--env=GOPROXY=http://172.30.0.1:45123", "--env=GOSUMDB=sum.golang.org", "--env=GOVCS=*:off"} {
		if !strings.Contains(joined, required) {
			t.Errorf("dependency container args missing %q", required)
		}
	}
	for _, forbidden := range []string{"--network=host", "--privileged", "--mount", "--volume", "--env=GOPROXY=direct", "--env=GOSUMDB=off"} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("dependency container args contain forbidden %q", forbidden)
		}
	}
}

type tarEntry struct {
	name     string
	body     string
	typeflag byte
	linkname string
}

func makeTar(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	for _, entry := range entries {
		typeflag := entry.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		header := &tar.Header{Name: entry.name, Typeflag: typeflag, Mode: 0o644, Size: int64(len(entry.body)), Linkname: entry.linkname}
		if typeflag != tar.TypeReg && typeflag != tar.TypeRegA {
			header.Size = 0
		}
		if err := w.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Size > 0 {
			if _, err := io.WriteString(w, entry.body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func readTarEntries(t *testing.T, data []byte) map[string]string {
	t.Helper()
	result := make(map[string]string)
	r := tar.NewReader(bytes.NewReader(data))
	for {
		h, err := r.Next()
		if err == io.EOF {
			return result
		}
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		result[h.Name] = string(body)
	}
}
