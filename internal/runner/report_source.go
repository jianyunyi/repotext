package runner

import (
	"bufio"
	"bytes"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"repotest/internal/domain"
)

var sourceDiagnostic = regexp.MustCompile(`(?m)^\s*(.+?\.go):([1-9][0-9]*)(?::[0-9]+)?(?:\s|:|$)`)
var reportCommit = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)

type reportSource struct {
	module   string
	files    map[string]int
	linkBase string
}

// sourceRoot is the already validated extracted archive, read before task cleanup.
// Index regular Go files only; failed validation leaves diagnostics usable without links.
func indexReportSource(sourceRoot string, task domain.Task) *reportSource {
	if sourceRoot == "" {
		return nil
	}
	root, err := filepath.Abs(sourceRoot)
	if err != nil {
		return nil
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil || resolved != root {
		return nil
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	source := &reportSource{files: make(map[string]int)}
	modInfo, err := os.Lstat(filepath.Join(root, "go.mod"))
	if err != nil || !modInfo.Mode().IsRegular() || modInfo.Size() > 1<<20 {
		return nil
	}
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(mod), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "module" {
			source.module = fields[1]
			if strings.HasPrefix(source.module, `"`) {
				source.module, _ = strconv.Unquote(source.module)
			}
			break
		}
	}
	entries := 0
	err = filepath.WalkDir(root, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		entries++
		if entries > 100000 {
			return ErrArchiveTooLarge
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return ErrUnsafeArchive
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return ErrUnsafeArchive
		}
		rel, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		file, err := os.Open(filename)
		if err != nil {
			return err
		}
		reader := bufio.NewReader(file)
		lines := 0
		lastNewline := true
		for {
			data, err := reader.ReadSlice('\n')
			lines += bytes.Count(data, []byte{'\n'})
			if len(data) > 0 {
				lastNewline = data[len(data)-1] == '\n'
			}
			if err == io.EOF {
				if !lastNewline {
					lines++
				}
				break
			}
			if err != nil && err != bufio.ErrBufferFull {
				_ = file.Close()
				return err
			}
		}
		if err := file.Close(); err != nil {
			return err
		}
		source.files[filepath.ToSlash(rel)] = lines
		return nil
	})
	if err != nil {
		return nil
	}
	if repo, err := ValidateRepoURL(task.RepoURL); err == nil && reportCommit.MatchString(task.Commit) {
		source.linkBase = repo + "/blob/" + task.Commit + "/"
	}
	return source
}

func (s *reportSource) location(pkg, output string) *domain.FailureLocation {
	if s == nil {
		return nil
	}
	for _, match := range sourceDiagnostic.FindAllStringSubmatch(output, -1) {
		filename := strings.TrimSpace(match[1])
		line, err := strconv.Atoi(match[2])
		if err != nil {
			continue
		}
		candidate, ok := s.relativeFile(pkg, filename)
		if !ok || line > s.files[candidate] {
			continue
		}
		location := &domain.FailureLocation{Path: candidate, Line: line}
		if s.linkBase != "" {
			parts := strings.Split(candidate, "/")
			for i := range parts {
				parts[i] = url.PathEscape(parts[i])
			}
			location.GitHubURL = s.linkBase + strings.Join(parts, "/") + "#L" + strconv.Itoa(line)
		}
		return location
	}
	return nil
}

func (s *reportSource) relativeFile(pkg, filename string) (string, bool) {
	if strings.ContainsAny(filename, ":\x00") {
		return "", false
	}
	if strings.Contains(filename, "\\") {
		// Go's Windows compiler also emits repository-relative paths. Convert only
		// that form; drive/UNC/host absolute paths never receive repository mapping.
		if strings.HasPrefix(filename, "\\") || strings.HasPrefix(filename, "/") {
			return "", false
		}
		filename = strings.ReplaceAll(filename, "\\", "/")
	}
	rootPath := false
	if strings.HasPrefix(filename, "/work/src/") {
		filename = strings.TrimPrefix(filename, "/work/src/")
		rootPath = true
	}
	if path.IsAbs(filename) {
		return "", false
	}
	filename = strings.TrimPrefix(filename, "./")
	for _, part := range strings.Split(filename, "/") {
		if part == "" || part == "." || part == ".." {
			return "", false
		}
	}
	if path.Clean(filename) != filename {
		return "", false
	}
	if s.module != "" && strings.HasPrefix(filename, s.module+"/") {
		filename = strings.TrimPrefix(filename, s.module+"/")
		rootPath = true
	}
	if !rootPath {
		if s.module == "" || (pkg != s.module && !strings.HasPrefix(pkg, s.module+"/")) {
			return "", false
		}
		dir := strings.TrimPrefix(pkg, s.module)
		dir = strings.TrimPrefix(dir, "/")
		if dir != "" {
			if path.Clean(dir) != dir || strings.ContainsAny(dir, "\\:\x00") {
				return "", false
			}
			for _, part := range strings.Split(dir, "/") {
				if part == "" || part == "." || part == ".." {
					return "", false
				}
			}
		}
		filename = path.Join(dir, filename)
	}
	_, ok := s.files[filename]
	return filename, ok
}
