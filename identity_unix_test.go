//go:build unix

package main

import (
	"os"
	"syscall"
	"testing"
)

func TestIsCrossDeviceUnwrapsLinkError(t *testing.T) {
	err := &os.LinkError{Op: "rename", Old: "a", New: "b", Err: syscall.EXDEV}
	if !isCrossDevice(err) {
		t.Fatal("wrapped EXDEV should be detected")
	}
	if isCrossDevice(os.ErrNotExist) {
		t.Fatal("unrelated error should not be treated as cross-device")
	}
}
