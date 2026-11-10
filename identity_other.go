//go:build !unix && !windows

package main

import (
	"os"
	"time"
)

func openNoFollow(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, errSymlink
	}
	if !info.Mode().IsRegular() {
		return nil, errNotRegular
	}
	return os.Open(path)
}

func identityFromInfo(info os.FileInfo) (dev, ino, nlink uint64, ok bool) {
	return 0, 0, 0, false
}

func identityFromFile(f *os.File) (dev, ino, nlink uint64, size int64, mtime time.Time, mode os.FileMode, err error) {
	info, err := f.Stat()
	if err != nil {
		return 0, 0, 0, 0, time.Time{}, 0, err
	}
	// Device/inode are unavailable; callers must treat identity as unknown.
	return 0, 0, 0, info.Size(), info.ModTime(), info.Mode(), nil
}

func isCrossDevice(err error) bool {
	return false
}
