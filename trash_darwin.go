//go:build darwin

package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type darwinFinderTrash struct{}

func (darwinFinderTrash) Name() string { return "finder" }

func (darwinFinderTrash) Available() bool {
	_, err := exec.LookPath("osascript")
	return err == nil
}

func (darwinFinderTrash) Trash(path string) (string, error) {
	if _, err := exec.LookPath("osascript"); err != nil {
		return "", errTrashUnsupported
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// Path is passed inside a quoted AppleScript string, never via a shell.
	script := "tell application \"Finder\" to delete POSIX file " + applescriptQuote(abs)
	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("finder trash: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return "Finder Trash", nil
}

func applescriptQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func platformTrash() trashBackend {
	t := darwinFinderTrash{}
	if !t.Available() {
		return unsupportedTrash{}
	}
	return t
}
