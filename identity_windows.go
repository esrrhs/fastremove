//go:build windows

package main

import (
	"errors"
	"os"
	"syscall"
	"time"
)

func openNoFollow(path string) (*os.File, error) {
	// Windows Open follows reparse points. We Lstat first and refuse
	// symlinks, but a replacement between Lstat and Open remains a
	// residual race (documented in the README).
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
	// Lstat Sys() is Win32FileAttributeData and has no file index.
	return 0, 0, 0, false
}

func identityFromFile(f *os.File) (dev, ino, nlink uint64, size int64, mtime time.Time, mode os.FileMode, err error) {
	info, err := f.Stat()
	if err != nil {
		return 0, 0, 0, 0, time.Time{}, 0, err
	}
	var d syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &d); err != nil {
		return 0, 0, 0, 0, time.Time{}, 0, err
	}
	dev = uint64(d.VolumeSerialNumber)
	ino = (uint64(d.FileIndexHigh) << 32) | uint64(d.FileIndexLow)
	nlink = uint64(d.NumberOfLinks)
	return dev, ino, nlink, info.Size(), info.ModTime(), info.Mode(), nil
}

func isCrossDevice(err error) bool {
	// ERROR_NOT_SAME_DEVICE (17). Compared via errors.Is so *os.LinkError unwraps.
	return errors.Is(err, syscall.Errno(17))
}
