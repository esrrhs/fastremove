//go:build unix

package main

import (
	"errors"
	"os"
	"syscall"
	"time"
)

func openNoFollow(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

func identityFromInfo(info os.FileInfo) (dev, ino, nlink uint64, ok bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, 0, false
	}
	return uint64(st.Dev), uint64(st.Ino), uint64(st.Nlink), true
}

func identityFromFile(f *os.File) (dev, ino, nlink uint64, size int64, mtime time.Time, mode os.FileMode, err error) {
	info, err := f.Stat()
	if err != nil {
		return 0, 0, 0, 0, time.Time{}, 0, err
	}
	dev, ino, nlink, ok := identityFromInfo(info)
	if !ok {
		return 0, 0, 0, 0, time.Time{}, 0, errNoIdentity
	}
	return dev, ino, nlink, info.Size(), info.ModTime(), info.Mode(), nil
}

func isCrossDevice(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}
