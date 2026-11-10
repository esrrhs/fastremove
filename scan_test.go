package main

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestUniqueSizeFilesAreNotHashed(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	unique := filepath.Join(dir, "unique.txt")
	mustWrite(t, a, "same-bytes")
	mustWrite(t, b, "same-bytes")
	mustWrite(t, unique, "different-length-content")

	var mu sync.Mutex
	var hashed []string
	prev := testOnHash
	testOnHash = func(path string) {
		mu.Lock()
		hashed = append(hashed, path)
		mu.Unlock()
	}
	t.Cleanup(func() { testOnHash = prev })

	groups, err := findDuplicates(dir, 2, "first")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}

	for _, p := range hashed {
		if filepath.Clean(p) == filepath.Clean(unique) {
			t.Fatalf("unique-size file %s was hashed: %v", unique, hashed)
		}
	}
	if len(hashed) != 2 {
		t.Fatalf("hashed %d files, want 2 (the same-size pair): %v", len(hashed), hashed)
	}
}

func TestSameSizeDifferentContentStillHashed(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "aaaa")
	mustWrite(t, filepath.Join(dir, "b.txt"), "bbbb") // same size, different content

	var mu sync.Mutex
	n := 0
	prev := testOnHash
	testOnHash = func(string) {
		mu.Lock()
		n++
		mu.Unlock()
	}
	t.Cleanup(func() { testOnHash = prev })

	groups, err := findDuplicates(dir, 2, "first")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("groups = %d, want 0", len(groups))
	}
	if n != 2 {
		t.Fatalf("hashed %d, want 2", n)
	}
}

func TestQuarantineDirExcludedFromScan(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "xx")
	mustWrite(t, filepath.Join(dir, "b.txt"), "xx")
	qdir := filepath.Join(dir, ".fastremove-quarantine", "session", "files")
	if err := os.MkdirAll(qdir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(qdir, "hidden.txt"), "xx")

	files, err := listFiles(dir, newQuarantineSkip())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if filepath.Base(f.path) == "hidden.txt" {
			t.Fatalf("quarantine file was scanned: %s", f.path)
		}
	}
	if len(files) != 2 {
		t.Fatalf("files = %d, want 2", len(files))
	}
}

func TestSymlinksAreNotScanned(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.txt")
	mustWrite(t, real, "data")
	if err := os.Symlink(real, filepath.Join(dir, "link.txt")); err != nil {
		t.Fatal(err)
	}
	files, err := listFiles(dir, scanSkip{})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].path != real {
		t.Fatalf("files = %+v, want only %s", files, real)
	}
}

func TestHardlinksAloneAreNotHashed(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	mustWrite(t, a, "shared-content")
	if err := os.Link(a, b); err != nil {
		t.Skipf("hard links not supported: %v", err)
	}

	var mu sync.Mutex
	n := 0
	prev := testOnHash
	testOnHash = func(string) {
		mu.Lock()
		n++
		mu.Unlock()
	}
	t.Cleanup(func() { testOnHash = prev })

	groups, err := findDuplicates(dir, 2, "first")
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("same-inode hard links should not be hashed, hashed=%d", n)
	}
	if len(groups) != 0 {
		t.Fatalf("groups = %d, want 0", len(groups))
	}
}
