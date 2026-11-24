//go:build linux || darwin

package session

import (
	"golang.org/x/sys/unix"
	"os"
)

func privatePlanFile(path string, create bool) (*os.File, error) {
	flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC
	if create {
		flags = unix.O_WRONLY | unix.O_CREAT | unix.O_EXCL | unix.O_NOFOLLOW | unix.O_CLOEXEC
	}
	fd, err := unix.Open(path, flags, 0600)
	if err != nil {
		return nil, err
	}
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || int(st.Uid) != os.Getuid() || st.Mode&0077 != 0 || st.Nlink != 1 {
		unix.Close(fd)
		return nil, ErrPlanFile
	}
	return os.NewFile(uintptr(fd), path), nil
}
