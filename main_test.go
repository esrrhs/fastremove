package main

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestRemoveDuplicateKeepsFirst(t *testing.T) {
	resetDuplicateState()

	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(a, []byte("same"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("same"), 0o644); err != nil {
		t.Fatal(err)
	}

	removeDuplicate(a, false, false)
	removeDuplicate(b, false, false)

	if atomic.LoadInt32(&gDuplicateNum) != 1 {
		t.Fatalf("duplicate count = %d, want 1", gDuplicateNum)
	}
	if _, err := os.Stat(a); err != nil {
		t.Fatalf("first file should remain: %v", err)
	}
	if _, err := os.Stat(b); err != nil {
		t.Fatalf("second file should remain without -delete: %v", err)
	}
}

func TestRemoveDuplicateDeletesWhenRequested(t *testing.T) {
	resetDuplicateState()

	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(a, []byte("dup"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("dup"), 0o644); err != nil {
		t.Fatal(err)
	}

	removeDuplicate(a, true, false)
	removeDuplicate(b, true, false)

	if _, err := os.Stat(a); err != nil {
		t.Fatalf("first file should remain: %v", err)
	}
	if _, err := os.Stat(b); !os.IsNotExist(err) {
		t.Fatalf("second file should be deleted, err=%v", err)
	}
}

func TestRemoveDuplicateIgnoresDifferentContent(t *testing.T) {
	resetDuplicateState()

	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(a, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}

	removeDuplicate(a, false, false)
	removeDuplicate(b, false, false)

	if atomic.LoadInt32(&gDuplicateNum) != 0 {
		t.Fatalf("duplicate count = %d, want 0", gDuplicateNum)
	}
}

func TestVersionNotEmpty(t *testing.T) {
	if Version == "" {
		t.Fatal("Version should not be empty")
	}
}

func resetDuplicateState() {
	gDuplicate.Range(func(key, _ any) bool {
		gDuplicate.Delete(key)
		return true
	})
	atomic.StoreInt32(&gDuplicateNum, 0)
}
