//go:build linux || darwin

package session

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"

	"golang.org/x/sys/unix"
)

func stateDirectory() (string, error) {
	account, err := user.LookupId(strconv.Itoa(os.Getuid()))
	if err != nil || !filepath.IsAbs(account.HomeDir) {
		return "", ErrUnsafeLock
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(account.HomeDir, "Library", "Application Support", "sa2-cli-go"), nil
	}
	return filepath.Join(account.HomeDir, ".local", "state", "sa2-cli-go"), nil
}

func openLock(dir, name string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	dirFD, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(dirFD)
	var info unix.Stat_t
	if err := unix.Fstat(dirFD, &info); err != nil || int(info.Uid) != os.Getuid() || info.Mode&0077 != 0 {
		return nil, ErrUnsafeLock
	}

	fd, err := unix.Openat(dirFD, name, unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0600)
	if err != nil {
		return nil, err
	}
	if err := unix.Fstat(fd, &info); err != nil || int(info.Uid) != os.Getuid() ||
		info.Mode&unix.S_IFMT != unix.S_IFREG || info.Mode&0077 != 0 || info.Nlink != 1 || info.Size != 0 {
		unix.Close(fd)
		return nil, ErrUnsafeLock
	}
	return os.NewFile(uintptr(fd), name), nil
}

func tryLock(file *os.File) (bool, error) {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EINTR) {
		return false, nil
	}
	return err == nil, err
}
