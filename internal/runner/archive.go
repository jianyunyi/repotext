package runner

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidArchiveTarget = errors.New("invalid archive target")
	ErrUnsafeArchive        = errors.New("unsafe repository archive")
	ErrArchiveTooLarge      = errors.New("repository archive exceeds configured limit")
)

var fullCommitSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

const (
	defaultArchiveBytes   = 100 << 20
	defaultArchiveEntries = 100000
)

type archiveLimits struct {
	maxCompressed int64
	maxExpanded   int64
	maxFileBytes  int64
	maxEntries    int
}

type ArchiveFetcher struct {
	client      *http.Client
	apiBase     string
	archiveHost string
	limits      archiveLimits
}

// NewArchiveFetcher creates a fetcher for public GitHub archives. It follows only
// one HTTPS redirect, and only when GitHub points at codeload.github.com.
func NewArchiveFetcher(client *http.Client) ArchiveFetcher {
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	return ArchiveFetcher{
		client:      client,
		apiBase:     "https://api.github.com",
		archiveHost: "codeload.github.com",
		limits: archiveLimits{
			maxCompressed: defaultArchiveBytes,
			maxExpanded:   defaultArchiveBytes,
			maxFileBytes:  defaultArchiveBytes,
			maxEntries:    defaultArchiveEntries,
		},
	}
}

// FetchAndExtract downloads the archive for an immutable commit SHA and extracts
// regular files/directories into a new destination below the caller's private task
// scratch parent. It does not execute repository contents. The destination must
// not already exist.
func (f ArchiveFetcher) FetchAndExtract(ctx context.Context, repoURL, commitSHA, destination string) error {
	canonicalRepo, err := ValidateRepoURL(repoURL)
	if err != nil || !fullCommitSHA.MatchString(commitSHA) {
		return ErrInvalidArchiveTarget
	}
	if err = f.validate(); err != nil {
		return err
	}
	if runtime.GOOS == "linux" {
		parent, err := os.Lstat(filepath.Dir(destination))
		if err != nil {
			return fmt.Errorf("inspect archive workspace parent: %w", err)
		}
		if !parent.IsDir() || parent.Mode().Perm()&0o077 != 0 {
			return errors.New("archive workspace parent must be a private directory")
		}
	}
	owner, repo := archiveRepoParts(canonicalRepo)
	endpoint := strings.TrimRight(f.apiBase, "/") + "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/tarball/" + strings.ToLower(commitSHA)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ErrInvalidArchiveTarget
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", "RepoTest-local/0.1")

	client := *f.client
	client.Jar = nil
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) != 1 || !f.allowedArchiveRedirect(next.URL, owner, repo, strings.ToLower(commitSHA)) {
			return fmt.Errorf("%w: redirect target rejected", ErrInvalidArchiveTarget)
		}
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Request == nil || !f.allowedArchiveRedirect(resp.Request.URL, owner, repo, strings.ToLower(commitSHA)) {
		return fmt.Errorf("%w: archive response rejected", ErrInvalidArchiveTarget)
	}
	if resp.ContentLength > f.limits.maxCompressed {
		return ErrArchiveTooLarge
	}

	if err = os.Mkdir(destination, 0700); err != nil {
		return err
	}
	if err = extractTarGz(resp.Body, destination, f.limits); err != nil {
		_ = os.RemoveAll(destination)
		return err
	}
	return nil
}

func (f ArchiveFetcher) validate() error {
	if f.client == nil || f.apiBase == "" || f.archiveHost == "" || f.limits.maxCompressed <= 0 || f.limits.maxExpanded <= 0 || f.limits.maxFileBytes <= 0 || f.limits.maxEntries <= 0 {
		return errors.New("invalid archive fetcher configuration")
	}
	api, err := url.Parse(f.apiBase)
	if err != nil || api.Scheme != "https" || api.Hostname() != "api.github.com" || api.User != nil || api.Port() != "" {
		return errors.New("archive API must use api.github.com over HTTPS")
	}
	if f.archiveHost != "codeload.github.com" {
		return errors.New("archive redirect host must be codeload.github.com")
	}
	return nil
}

func (f ArchiveFetcher) allowedArchiveRedirect(u *url.URL, owner, repo, sha string) bool {
	if u == nil || u.Scheme != "https" || !strings.EqualFold(u.Hostname(), f.archiveHost) || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	prefix := "/" + owner + "/" + repo + "/"
	if !strings.HasPrefix(u.EscapedPath(), prefix) {
		return false
	}
	return strings.EqualFold(path.Base(u.Path), sha)
}

func archiveRepoParts(repo string) (string, string) {
	parts := strings.Split(strings.TrimPrefix(repo, "https://github.com/"), "/")
	return parts[0], strings.TrimSuffix(parts[1], ".git")
}

func extractTarGz(input io.Reader, destination string, limits archiveLimits) (err error) {
	if limits.maxCompressed <= 0 || limits.maxExpanded <= 0 || limits.maxFileBytes <= 0 || limits.maxEntries <= 0 {
		return errors.New("invalid archive limits")
	}
	info, err := os.Lstat(destination)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: destination must be a real directory", ErrUnsafeArchive)
	}
	contents, err := os.ReadDir(destination)
	if err != nil {
		return err
	}
	if len(contents) != 0 {
		return fmt.Errorf("%w: destination is not empty", ErrUnsafeArchive)
	}
	defer func() {
		if err != nil {
			clearDirectory(destination)
		}
	}()

	compressed := &countingReader{reader: io.LimitReader(input, limits.maxCompressed+1)}
	zipper, err := gzip.NewReader(compressed)
	if err != nil {
		return fmt.Errorf("%w: invalid gzip stream", ErrUnsafeArchive)
	}
	expanded := &countingReader{reader: io.LimitReader(zipper, limits.maxExpanded+1)}
	reader := tar.NewReader(expanded)
	root := ""
	seenFiles := make(map[string]struct{})
	entries := 0
	payloadBytes := int64(0)
	for {
		header, readErr := reader.Next()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return fmt.Errorf("%w: invalid tar entry", ErrUnsafeArchive)
		}
		if header.Typeflag == tar.TypeXGlobalHeader {
			if err = validateGlobalPAXHeader(reader, header); err != nil {
				return err
			}
			continue
		}
		entries++
		if entries > limits.maxEntries {
			return ErrArchiveTooLarge
		}
		relative, entryRoot, pathErr := archiveRelativePath(header.Name, root)
		if pathErr != nil {
			return pathErr
		}
		if root == "" {
			root = entryRoot
		}
		if relative == "" {
			if header.Typeflag != tar.TypeDir || header.Size != 0 {
				return fmt.Errorf("%w: invalid archive root entry", ErrUnsafeArchive)
			}
			continue
		}
		pathOnDisk := filepath.Join(destination, filepath.FromSlash(relative))
		if !pathWithin(destination, pathOnDisk) {
			return fmt.Errorf("%w: archive path escaped destination", ErrUnsafeArchive)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if header.Size != 0 {
				return fmt.Errorf("%w: directory entry has data", ErrUnsafeArchive)
			}
			if err = os.MkdirAll(pathOnDisk, 0755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || header.Size > limits.maxFileBytes-payloadBytes {
				return ErrArchiveTooLarge
			}
			if _, exists := seenFiles[relative]; exists {
				return fmt.Errorf("%w: duplicate archive file", ErrUnsafeArchive)
			}
			if err = os.MkdirAll(filepath.Dir(pathOnDisk), 0755); err != nil {
				return err
			}
			mode := os.FileMode(0644)
			if os.FileMode(header.Mode)&0111 != 0 {
				mode |= 0111
			}
			file, openErr := os.OpenFile(pathOnDisk, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
			if openErr != nil {
				return openErr
			}
			_, copyErr := io.CopyN(file, reader, header.Size)
			closeErr := file.Close()
			if copyErr != nil {
				return fmt.Errorf("%w: truncated archive file", ErrUnsafeArchive)
			}
			if closeErr != nil {
				return closeErr
			}
			payloadBytes += header.Size
			seenFiles[relative] = struct{}{}
		default:
			return fmt.Errorf("%w: links and special files are not allowed", ErrUnsafeArchive)
		}
	}
	if root == "" {
		return fmt.Errorf("%w: empty archive", ErrUnsafeArchive)
	}
	_, err = io.Copy(io.Discard, expanded)
	if err != nil {
		return fmt.Errorf("%w: decompression failed", ErrUnsafeArchive)
	}
	if expanded.count > limits.maxExpanded || compressed.count > limits.maxCompressed {
		return ErrArchiveTooLarge
	}
	if err = zipper.Close(); err != nil {
		return fmt.Errorf("%w: invalid gzip trailer", ErrUnsafeArchive)
	}
	if err = os.Chmod(destination, 0755); err != nil {
		return err
	}
	return nil
}

func validateGlobalPAXHeader(reader io.Reader, header *tar.Header) error {
	if header.Name != "pax_global_header" || header.Size < 0 || header.Size > 64<<10 {
		return fmt.Errorf("%w: unsupported global PAX header", ErrUnsafeArchive)
	}
	body, err := io.ReadAll(io.LimitReader(reader, header.Size+1))
	if err != nil || int64(len(body)) != header.Size {
		return fmt.Errorf("%w: truncated global PAX header", ErrUnsafeArchive)
	}
	for offset := 0; offset < len(body); {
		space := bytes.IndexByte(body[offset:], ' ')
		if space <= 0 {
			return fmt.Errorf("%w: malformed global PAX record", ErrUnsafeArchive)
		}
		space += offset
		length, parseErr := strconv.Atoi(string(body[offset:space]))
		if parseErr != nil || length <= space-offset || offset+length > len(body) || body[offset+length-1] != '\n' {
			return fmt.Errorf("%w: malformed global PAX record", ErrUnsafeArchive)
		}
		record := string(body[space+1 : offset+length-1])
		key, _, found := strings.Cut(record, "=")
		if !found || key != "comment" {
			return fmt.Errorf("%w: unsupported global PAX metadata", ErrUnsafeArchive)
		}
		offset += length
	}
	return nil
}

func archiveRelativePath(raw, currentRoot string) (string, string, error) {
	if raw == "" || strings.Contains(raw, `\`) || strings.HasPrefix(raw, "/") || strings.Contains(raw, "//") {
		return "", "", fmt.Errorf("%w: invalid archive path", ErrUnsafeArchive)
	}
	trimmed := strings.TrimSuffix(raw, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 {
		return "", "", fmt.Errorf("%w: invalid archive path", ErrUnsafeArchive)
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", "", fmt.Errorf("%w: path traversal entry", ErrUnsafeArchive)
		}
	}
	root := parts[0]
	if currentRoot != "" && root != currentRoot {
		return "", "", fmt.Errorf("%w: multiple archive roots", ErrUnsafeArchive)
	}
	if len(parts) == 1 {
		return "", root, nil
	}
	return strings.Join(parts[1:], "/"), root, nil
}

func pathWithin(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func clearDirectory(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		_ = os.RemoveAll(filepath.Join(dir, entry.Name()))
	}
}

type countingReader struct {
	reader io.Reader
	count  int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.count += int64(n)
	return n, err
}
