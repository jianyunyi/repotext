package runner

import (
	"archive/tar"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"repotest/internal/domain"
)

var imageID = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

const offlineCommand = `set -eu
mkdir -p /work/src /work/cache /work/modules /work/tmp
tar -xf - -C /work
cd /work/src
test -f go.mod
exec go test -json -cover -count=1 -mod=readonly -p=2 -parallel=2 ./...`

const moduleDownloadCommand = `set -eu
mkdir -p /work/src /work/cache /work/modules /work/tmp
tar -xf - -C /work/src
cd /work/src
test -f go.mod
go mod download all
tar -cf - -C /work/modules .`

// runOfflineGo runs a staged workspace in a disposable, disconnected container.
// The image ID must identify a locally installed, trusted Go toolchain image.
func runOfflineGo(ctx context.Context, image, source string) (result Result, err error) {
	return runOfflineGoWithPhases(ctx, image, source, nil)
}

func runOfflineGoWithPhases(ctx context.Context, image, source string, setPhase func(domain.Phase) error) (result Result, err error) {
	if runtime.GOOS != "linux" || !imageID.MatchString(image) {
		return result, errors.New("offline runner requires Linux and a local Go image ID")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if setPhase != nil {
		if err = setPhase(domain.PhaseDeps); err != nil {
			return result, err
		}
	}
	scratch, err := os.MkdirTemp(filepath.Dir(source), "modules-")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(scratch)
	modulesTar, dependencyOutput, dependencyTruncated, err := prepareModuleCache(ctx, image, source, scratch)
	if err != nil {
		result.Output, result.Truncated = dependencyOutput, dependencyTruncated
		return result, err
	}
	if setPhase != nil {
		if err = setPhase(domain.PhaseTest); err != nil {
			result.Output, result.Truncated = dependencyOutput, dependencyTruncated
			return result, err
		}
	}
	result, err = runOfflineGoWithModules(ctx, image, source, modulesTar)
	result.Output, result.Truncated = joinLimitedOutput(result.Output, dependencyOutput, 2<<20, result.Truncated || dependencyTruncated)
	return result, err
}

func runOfflineGoWithModules(ctx context.Context, image, source, modulesTar string) (result Result, err error) {
	if runtime.GOOS != "linux" || !imageID.MatchString(image) {
		return result, errors.New("offline runner requires Linux and a local Go image ID")
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return result, err
	}
	name := fmt.Sprintf("repotest-offline-%x", nonce)

	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		output, cleanupErr := exec.CommandContext(cleanupCtx, "docker", "container", "rm", "-f", name).CombinedOutput()
		if cleanupErr != nil && !strings.Contains(string(output), "No such container: "+name) {
			err = errors.Join(err, fmt.Errorf("remove offline container %s: %w", name, cleanupErr))
		}
	}()

	createCtx, stopCreate := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	output, createErr := exec.CommandContext(createCtx, "docker", "create", "-i", "--pull=never",
		"--name="+name, "--label="+dockerManagedLabelValue(), "--label=io.repotest.kind=offline-test", "--rm", "--entrypoint=/bin/sh",
		"--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges",
		"--user=65534:65534", "--memory=768m", "--memory-swap=768m", "--cpus=1", "--pids-limit=128",
		"--log-driver=none", "--shm-size=16m",
		"--tmpfs=/work:rw,exec,nosuid,nodev,size=384m,mode=1777",
		"--tmpfs=/tmp:rw,noexec,nosuid,nodev,size=32m,mode=1777",
		"--env=HOME=/tmp", "--env=GOENV=off", "--env=GOWORK=off", "--env=GOFLAGS=", "--env=GOTOOLCHAIN=local",
		"--env=CGO_ENABLED=0", "--env=GOPROXY=off", "--env=GOVCS=*:off",
		"--env=GOCACHE=/work/cache", "--env=GOMODCACHE=/work/modules", "--env=GOTMPDIR=/work/tmp", "--env=GOMAXPROCS=2",
		image, "-c", offlineCommand).CombinedOutput()
	stopCreate()
	if createErr != nil {
		return result, fmt.Errorf("create offline container: %w: %s", createErr, strings.TrimSpace(string(output)))
	}

	input, writer := io.Pipe()
	writeDone := make(chan error, 1)
	go func() {
		moduleFile, openErr := os.Open(modulesTar)
		if openErr != nil {
			_ = writer.CloseWithError(openErr)
			writeDone <- openErr
			return
		}
		defer moduleFile.Close()
		writeErr := writeWorkspaceBundle(writer, source, moduleFile)
		_ = writer.CloseWithError(writeErr)
		writeDone <- writeErr
	}()
	cmd := exec.CommandContext(ctx, "docker", "start", "-ai", name)
	cmd.WaitDelay = 3 * time.Second
	cmd.Stdin = input
	var log limitedBuffer
	log.limit = 2 << 20
	cmd.Stdout, cmd.Stderr = &log, &log
	started := time.Now()
	startErr := cmd.Run()
	_ = input.Close()
	writeErr := <-writeDone
	result = Result{Duration: time.Since(started), Output: log.buf.Bytes(), Truncated: log.truncated}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if writeErr != nil {
		return result, fmt.Errorf("stream offline workspace: %w", errors.Join(writeErr, startErr))
	}
	if startErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(startErr, &exitErr) {
			return result, startErr
		}
		result.ExitCode = exitErr.ExitCode()
	}
	return result, nil
}

func prepareModuleCache(ctx context.Context, image, source, scratch string) (moduleTar string, output []byte, truncated bool, err error) {
	if runtime.GOOS != "linux" || !imageID.MatchString(image) {
		return "", nil, false, errors.New("dependency preparation requires Linux and a local Go image ID")
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return "", nil, false, err
	}
	suffix := fmt.Sprintf("%x", nonce)
	network := "repotest-deps-" + suffix
	container := "repotest-modules-" + suffix
	if out, createErr := exec.CommandContext(ctx, "docker", "network", "create", "--internal", "--label="+dockerManagedLabelValue(), "--label=io.repotest.kind=module-preparation", network).CombinedOutput(); createErr != nil {
		return "", nil, false, fmt.Errorf("create isolated module network: %w: %s", createErr, strings.TrimSpace(string(out)))
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, cleanupErr := exec.CommandContext(cleanupCtx, "docker", "network", "rm", network).CombinedOutput()
		if cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("remove module network: %w", cleanupErr))
		}
	}()
	inspect, err := exec.CommandContext(ctx, "docker", "network", "inspect", "--format={{range .IPAM.Config}}{{.Gateway}} {{.Subnet}}{{end}}", network).CombinedOutput()
	if err != nil {
		return "", nil, false, fmt.Errorf("inspect isolated module network: %w: %s", err, strings.TrimSpace(string(inspect)))
	}
	fields := strings.Fields(string(inspect))
	if len(fields) != 2 {
		return "", nil, false, errors.New("module network did not return one gateway and subnet")
	}
	gateway := net.ParseIP(fields[0])
	subnet, parseErr := netip.ParsePrefix(fields[1])
	if gateway == nil || parseErr != nil || !subnet.Contains(netip.MustParseAddr(gateway.String())) {
		return "", nil, false, errors.New("module network gateway is invalid")
	}
	proxy := newPublicModuleProxy()
	listener, err := net.Listen("tcp", net.JoinHostPort(gateway.String(), "0"))
	if err != nil {
		return "", nil, false, fmt.Errorf("bind module proxy to isolated network gateway: %w", err)
	}
	defer listener.Close()
	proxyServer := &http.Server{Handler: moduleNetworkOnly{next: proxy, subnet: subnet}, ReadHeaderTimeout: 3 * time.Second, IdleTimeout: 20 * time.Second}
	proxyDone := make(chan error, 1)
	go func() { proxyDone <- proxyServer.Serve(listener) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = proxyServer.Shutdown(shutdownCtx)
		serveErr := <-proxyDone
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			err = errors.Join(err, serveErr)
		}
	}()

	proxyAddr := listener.Addr().(*net.TCPAddr)
	args := dependencyContainerArgs(network, "http://"+net.JoinHostPort(gateway.String(), fmt.Sprint(proxyAddr.Port)))
	args = append(args, "--name="+container, "--label="+dockerManagedLabelValue(), "--label=io.repotest.kind=module-preparation", "--entrypoint=/bin/sh", image, "-c", moduleDownloadCommand)
	createCtx, stopCreate := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	created, createErr := exec.CommandContext(createCtx, "docker", append([]string{"create"}, args...)...).CombinedOutput()
	stopCreate()
	if createErr != nil {
		return "", nil, false, fmt.Errorf("create dependency container: %w: %s", createErr, strings.TrimSpace(string(created)))
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		removed, removeErr := exec.CommandContext(cleanupCtx, "docker", "container", "rm", "-f", container).CombinedOutput()
		if removeErr != nil && !strings.Contains(string(removed), "No such container: "+container) {
			err = errors.Join(err, fmt.Errorf("remove dependency container: %w", removeErr))
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	moduleTar = filepath.Join(scratch, "modules.tar")
	file, err := os.OpenFile(moduleTar, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", nil, false, err
	}
	input, writer := io.Pipe()
	writeDone := make(chan error, 1)
	go func() {
		writeErr := writeWorkspaceTar(writer, source)
		_ = writer.CloseWithError(writeErr)
		writeDone <- writeErr
	}()
	start := exec.CommandContext(ctx, "docker", "start", "-ai", container)
	start.WaitDelay = 3 * time.Second
	start.Stdin = input
	var logs limitedBuffer
	logs.limit = 2 << 20
	moduleOutput := &cappedFileWriter{file: file, limit: 512 << 20}
	start.Stdout, start.Stderr = moduleOutput, &logs
	startErr := start.Run()
	_ = input.Close()
	writeErr := <-writeDone
	closeErr := file.Close()
	output = logs.buf.Bytes()
	truncated = logs.truncated
	if ctx.Err() != nil {
		_ = os.Remove(moduleTar)
		return "", output, truncated, ctx.Err()
	}
	if writeErr != nil || startErr != nil || closeErr != nil {
		_ = os.Remove(moduleTar)
		return "", output, truncated, fmt.Errorf("download Go modules: %w: %s", errors.Join(writeErr, startErr, closeErr), strings.TrimSpace(string(output)))
	}
	return moduleTar, output, truncated, nil
}

func dependencyContainerArgs(network, proxy string) []string {
	return []string{
		"-i", "--pull=never", "--network=" + network, "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges",
		"--user=65534:65534", "--memory=768m", "--memory-swap=768m", "--cpus=1", "--pids-limit=96",
		"--log-driver=none", "--shm-size=16m", "--tmpfs=/work:rw,exec,nosuid,nodev,size=640m,mode=1777",
		"--tmpfs=/tmp:rw,noexec,nosuid,nodev,size=32m,mode=1777", "--env=HOME=/tmp", "--env=GOENV=off",
		"--env=GOWORK=off", "--env=GOFLAGS=", "--env=GOTOOLCHAIN=local", "--env=CGO_ENABLED=0", "--env=GOPROXY=" + proxy,
		"--env=GOSUMDB=sum.golang.org", "--env=GONOSUMDB=", "--env=GOPRIVATE=", "--env=GONOPROXY=",
		"--env=GOINSECURE=", "--env=GOVCS=*:off", "--env=GOCACHE=/work/cache", "--env=GOMODCACHE=/work/modules",
		"--env=GOTMPDIR=/work/tmp", "--env=GOMAXPROCS=2",
	}
}

type moduleNetworkOnly struct {
	next   http.Handler
	subnet netip.Prefix
}

func (h moduleNetworkOnly) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	ip, parseErr := netip.ParseAddr(host)
	if err != nil || parseErr != nil || !h.subnet.Contains(ip) {
		http.Error(w, "module proxy client is outside the task network", http.StatusForbidden)
		return
	}
	h.next.ServeHTTP(w, r)
}

type cappedFileWriter struct {
	file  *os.File
	limit int64
	size  int64
}

func (w *cappedFileWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > w.limit-w.size {
		return 0, errors.New("module cache export exceeds byte limit")
	}
	n, err := w.file.Write(data)
	w.size += int64(n)
	return n, err
}

func joinLimitedOutput(first, second []byte, limit int, alreadyTruncated bool) ([]byte, bool) {
	var out limitedBuffer
	out.limit = limit
	_, _ = out.Write(first)
	_, _ = out.Write(second)
	return out.buf.Bytes(), alreadyTruncated || out.truncated
}

func writeWorkspaceTar(output io.Writer, source string) error {
	return writeWorkspaceAt(output, source, "")
}

func writeWorkspaceBundle(output io.Writer, source string, moduleTar io.Reader) error {
	archive := tar.NewWriter(output)
	err := writeWorkspaceAtWriter{archive: archive, source: source, prefix: "src"}.write()
	if err != nil {
		_ = archive.Close()
		return err
	}
	reader := tar.NewReader(moduleTar)
	seen := make(map[string]struct{})
	var total int64
	entries := 0
	for {
		header, nextErr := reader.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			_ = archive.Close()
			return nextErr
		}
		entries++
		if entries > 100000 {
			_ = archive.Close()
			return ErrArchiveTooLarge
		}
		name := strings.TrimSuffix(header.Name, "/")
		name = strings.TrimPrefix(name, "./")
		if name == "" || name == "." || name == "modules" {
			if header.Typeflag == tar.TypeDir && header.Size == 0 {
				continue
			}
			_ = archive.Close()
			return fmt.Errorf("%w: invalid module cache root", ErrUnsafeArchive)
		}
		name = strings.TrimPrefix(name, "modules/")
		if name == "" || path.IsAbs(name) || strings.Contains(name, `\`) || strings.Contains(name, "//") || path.Clean(name) != name {
			_ = archive.Close()
			return fmt.Errorf("%w: unsafe module cache path", ErrUnsafeArchive)
		}
		for _, part := range strings.Split(name, "/") {
			if part == "" || part == "." || part == ".." {
				_ = archive.Close()
				return fmt.Errorf("%w: module cache path traversal", ErrUnsafeArchive)
			}
		}
		name = "modules/" + name
		if _, exists := seen[name]; exists {
			_ = archive.Close()
			return fmt.Errorf("%w: duplicate module cache entry", ErrUnsafeArchive)
		}
		seen[name] = struct{}{}
		switch header.Typeflag {
		case tar.TypeDir:
			if header.Size != 0 {
				_ = archive.Close()
				return fmt.Errorf("%w: module cache directory contains data", ErrUnsafeArchive)
			}
			header.Name, header.Mode = name, 0755
			if err := archive.WriteHeader(header); err != nil {
				_ = archive.Close()
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || header.Size > (512<<20)-total {
				_ = archive.Close()
				return ErrArchiveTooLarge
			}
			total += header.Size
			header.Name, header.Mode = name, 0644
			if err := archive.WriteHeader(header); err != nil {
				_ = archive.Close()
				return err
			}
			if _, err := io.CopyN(archive, reader, header.Size); err != nil {
				_ = archive.Close()
				return fmt.Errorf("%w: truncated module cache entry", ErrUnsafeArchive)
			}
		default:
			_ = archive.Close()
			return fmt.Errorf("%w: module cache links and special files are forbidden", ErrUnsafeArchive)
		}
	}
	return archive.Close()
}

func writeWorkspaceAt(output io.Writer, source, prefix string) error {
	archive := tar.NewWriter(output)
	err := (writeWorkspaceAtWriter{archive: archive, source: source, prefix: prefix}).write()
	if err != nil {
		_ = archive.Close()
		return err
	}
	return archive.Close()
}

type writeWorkspaceAtWriter struct {
	archive *tar.Writer
	source  string
	prefix  string
}

func (w writeWorkspaceAtWriter) write() error {
	return filepath.WalkDir(w.source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == w.source {
			if !entry.IsDir() {
				return errors.New("offline workspace must be a directory")
			}
			if w.prefix != "" {
				return w.archive.WriteHeader(&tar.Header{Name: w.prefix + "/", Typeflag: tar.TypeDir, Mode: 0755})
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return errors.New("offline workspace contains a special file")
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(w.source, path)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		if w.prefix != "" {
			header.Name = w.prefix + "/" + header.Name
		}
		header.Mode &^= 0o7000
		if err := w.archive.WriteHeader(header); err != nil || info.IsDir() {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.CopyN(w.archive, file, info.Size())
		return err
	})
}
