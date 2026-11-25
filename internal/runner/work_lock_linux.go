//go:build linux

package runner

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
)

type RunnerWorkLock struct {
	file *os.File
}

func RunnerServiceLockFile() (string, error) {
	uid := strconv.Itoa(syscall.Getuid())
	account, err := user.LookupId(uid)
	if err != nil || !filepath.IsAbs(account.HomeDir) {
		return "", errors.New("runner requires a stable OS user home for its service lock")
	}
	homeInfo, err := os.Lstat(account.HomeDir)
	if err != nil {
		return "", err
	}
	homeOwner, ok := homeInfo.Sys().(*syscall.Stat_t)
	if !homeInfo.IsDir() || homeInfo.Mode()&os.ModeSymlink != 0 || homeInfo.Mode().Perm()&0o022 != 0 || !ok || homeOwner.Uid != uint32(syscall.Getuid()) {
		return "", errors.New("runner OS user home must be a real directory owned by the current user")
	}
	dir := account.HomeDir
	for _, part := range []string{".local", "state", "repotest"} {
		dir = filepath.Join(dir, part)
		if err = os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return "", err
		}
		info, statErr := os.Lstat(dir)
		if statErr != nil {
			return "", statErr
		}
		owner, ownerOK := info.Sys().(*syscall.Stat_t)
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 || !ownerOK || owner.Uid != uint32(syscall.Getuid()) {
			return "", errors.New("runner state path must contain only real directories owned by the current user")
		}
	}
	info, err := os.Lstat(dir)
	if err != nil || info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("runner state directory must be private")
	}
	return filepath.Join(dir, "runner.lock"), nil
}

func dockerManagedLabelValue() string {
	return "io.repotest.owner=repotest-s4-v1-uid-" + strconv.Itoa(syscall.Getuid())
}

func AcquireRunnerWorkLock(lockPath string) (*RunnerWorkLock, error) {
	if !filepath.IsAbs(lockPath) {
		return nil, errors.New("runner lock path must be absolute")
	}
	lockDir := filepath.Dir(lockPath)
	dirInfo, err := os.Lstat(lockDir)
	if err != nil || !dirInfo.IsDir() || dirInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("runner lock directory must be a real directory")
	}
	dirOwner, dirOwnerOK := dirInfo.Sys().(*syscall.Stat_t)
	privateOwnerDir := dirOwnerOK && dirOwner.Uid == uint32(syscall.Getuid()) && dirInfo.Mode().Perm()&0o077 == 0
	stickySharedDir := dirInfo.Mode()&os.ModeSticky != 0 && dirInfo.Mode().Perm()&0o002 != 0
	if !privateOwnerDir && !stickySharedDir {
		return nil, errors.New("runner lock directory must be private or sticky shared storage")
	}
	fd, err := syscall.Open(lockPath, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), lockPath)
	if err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, errors.New("runner instance is already active for this OS user")
	}
	lockInfo, statErr := os.Lstat(lockPath)
	if statErr != nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return nil, statErr
	}
	lockOwner, lockOwnerOK := lockInfo.Sys().(*syscall.Stat_t)
	if !lockInfo.Mode().IsRegular() || lockInfo.Mode()&os.ModeSymlink != 0 || !lockOwnerOK || lockOwner.Uid != uint32(syscall.Getuid()) {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return nil, errors.New("runner lock file must be owned by the current user")
	}
	if err = syscall.Fchmod(fd, 0o600); err != nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return nil, err
	}
	return &RunnerWorkLock{file: file}, nil
}

func (l *RunnerWorkLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	l.file = nil
	return errors.Join(unlockErr, closeErr)
}
