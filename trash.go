package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var errTrashUnsupported = errors.New("trash backend unavailable")

// trashBackend moves a file into an OS trash/recycle bin.
// Implementations must not invoke a shell and must pass paths as literal
// arguments or write the freedesktop trash format directly.
type trashBackend interface {
	Name() string
	Available() bool
	// Trash moves path out of place. recovery is a hint the user can follow.
	Trash(path string) (recovery string, err error)
}

type unsupportedTrash struct{}

func (unsupportedTrash) Name() string    { return "none" }
func (unsupportedTrash) Available() bool { return false }
func (unsupportedTrash) Trash(string) (string, error) {
	return "", errTrashUnsupported
}

// failingTrash is a test double whose Trash always returns err.
type failingTrash struct{ err error }

func (f failingTrash) Name() string    { return "failing" }
func (f failingTrash) Available() bool { return true }
func (f failingTrash) Trash(string) (string, error) {
	if f.err == nil {
		return "", errors.New("trash failed")
	}
	return "", f.err
}

// scriptedTrash is a test double with an injectable function.
type scriptedTrash struct {
	name string
	fn   func(path string) (string, error)
}

func (s scriptedTrash) Name() string    { return s.name }
func (s scriptedTrash) Available() bool { return true }
func (s scriptedTrash) Trash(path string) (string, error) {
	return s.fn(path)
}

// freedesktopTrash implements the XDG trash spec under a given root
// (normally $XDG_DATA_HOME/Trash). Tests inject a temporary root.
type freedesktopTrash struct {
	root string
}

func (t freedesktopTrash) Name() string { return "freedesktop" }

func (t freedesktopTrash) Available() bool {
	if t.root == "" {
		return false
	}
	if err := os.MkdirAll(filepath.Join(t.root, "files"), 0o700); err != nil {
		return false
	}
	if err := os.MkdirAll(filepath.Join(t.root, "info"), 0o700); err != nil {
		return false
	}
	return true
}

func (t freedesktopTrash) Trash(path string) (string, error) {
	if t.root == "" {
		return "", errTrashUnsupported
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	filesDir := filepath.Join(t.root, "files")
	infoDir := filepath.Join(t.root, "info")
	if err := os.MkdirAll(filesDir, 0o700); err != nil {
		return "", err
	}
	if err := os.MkdirAll(infoDir, 0o700); err != nil {
		return "", err
	}

	base := filepath.Base(abs)
	if base == "" || base == "." || base == ".." {
		return "", fmt.Errorf("invalid trash name for %s", abs)
	}
	dest, infoPath, err := uniqueTrashNames(filesDir, infoDir, base)
	if err != nil {
		return "", err
	}

	infoBody := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n",
		url.QueryEscape(abs), time.Now().Format("2006-01-02T15:04:05"))
	if err := os.WriteFile(infoPath, []byte(infoBody), 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(abs, dest); err != nil {
		_ = os.Remove(infoPath)
		if isCrossDevice(err) {
			return "", fmt.Errorf("trash is on a different device: %w", err)
		}
		return "", err
	}
	return dest, nil
}

func uniqueTrashNames(filesDir, infoDir, base string) (dest, infoPath string, err error) {
	for i := 0; i < 10000; i++ {
		name := base
		if i > 0 {
			ext := filepath.Ext(base)
			stem := strings.TrimSuffix(base, ext)
			name = fmt.Sprintf("%s_%d%s", stem, i, ext)
		}
		dest = filepath.Join(filesDir, name)
		infoPath = filepath.Join(infoDir, name+".trashinfo")
		if _, err1 := os.Lstat(dest); err1 == nil {
			continue
		} else if !os.IsNotExist(err1) {
			return "", "", err1
		}
		if _, err2 := os.Lstat(infoPath); err2 == nil {
			continue
		} else if !os.IsNotExist(err2) {
			return "", "", err2
		}
		return dest, infoPath, nil
	}
	return "", "", fmt.Errorf("too many trash name collisions for %s", base)
}

func defaultFreedesktopTrashRoot() string {
	if v := os.Getenv("FASTREMOVE_TRASH_HOME"); v != "" {
		return filepath.Join(v, "Trash")
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "Trash")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "share", "Trash")
}
