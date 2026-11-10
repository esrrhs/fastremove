package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func scanPair(t *testing.T, dir, keepName, candName, content string) (fileMeta, fileMeta) {
	t.Helper()
	keep := filepath.Join(dir, keepName)
	cand := filepath.Join(dir, candName)
	mustWrite(t, keep, content)
	mustWrite(t, cand, content)
	groups, err := findDuplicates(dir, 2, "first")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].remove) != 1 {
		t.Fatalf("unexpected groups: %+v", groups)
	}
	return groups[0].keep, groups[0].remove[0]
}

func TestRevalidateOK(t *testing.T) {
	dir := t.TempDir()
	keep, cand := scanPair(t, dir, "a.txt", "b.txt", "payload")
	if err := revalidatePair(keep, cand); err != nil {
		t.Fatal(err)
	}
}

func TestRevalidateDeletedKeeper(t *testing.T) {
	dir := t.TempDir()
	keep, cand := scanPair(t, dir, "a.txt", "b.txt", "payload")
	if err := os.Remove(keep.path); err != nil {
		t.Fatal(err)
	}
	err := revalidatePair(keep, cand)
	if !errors.Is(err, errMissing) {
		t.Fatalf("err = %v, want missing keeper", err)
	}

	cfg := testRemoveCfg(t)
	groups := []duplicateGroup{{hash: keep.hash, keep: keep, remove: []fileMeta{cand}}}
	stats := processRemovals(groups, cfg)
	if stats.Skipped != 1 || stats.Quarantined != 0 {
		t.Fatalf("stats = %+v, want candidate skipped", stats)
	}
	if _, err := os.Stat(cand.path); err != nil {
		t.Fatalf("candidate should remain when keeper is gone: %v", err)
	}
}

func TestRevalidateMutatedCandidate(t *testing.T) {
	dir := t.TempDir()
	keep, cand := scanPair(t, dir, "a.txt", "b.txt", "payload")
	mustWrite(t, cand.path, "changed-content")
	err := revalidatePair(keep, cand)
	if !errors.Is(err, errModified) && !errors.Is(err, errContentMismatch) {
		t.Fatalf("err = %v, want modified or content mismatch", err)
	}

	cfg := testRemoveCfg(t)
	stats := processRemovals([]duplicateGroup{{hash: keep.hash, keep: keep, remove: []fileMeta{cand}}}, cfg)
	if stats.Skipped != 1 {
		t.Fatalf("stats = %+v, want skip", stats)
	}
	if _, err := os.Stat(cand.path); err != nil {
		t.Fatalf("mutated candidate should remain: %v", err)
	}
}

func TestRevalidateReplacedCandidate(t *testing.T) {
	dir := t.TempDir()
	keep, cand := scanPair(t, dir, "a.txt", "b.txt", "payload")
	info, err := os.Lstat(cand.path)
	if err != nil {
		t.Fatal(err)
	}
	mtime := info.ModTime()
	oldIno := cand.ino
	if err := os.Remove(cand.path); err != nil {
		t.Fatal(err)
	}
	// Occupy the freed inode so the recreation cannot reuse it.
	mustWrite(t, filepath.Join(dir, "inode-filler"), "payload")
	mustWrite(t, cand.path, "payload")
	if err := os.Chtimes(cand.path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	live, err := lstatLive(cand.path)
	if err != nil {
		t.Fatal(err)
	}
	if live.idOK && cand.idOK && live.ino == oldIno {
		t.Skip("filesystem reused inode; cannot construct a replaced-file case")
	}
	err = revalidatePair(keep, cand)
	if !errors.Is(err, errReplaced) {
		t.Fatalf("err = %v, want replaced", err)
	}

	cfg := testRemoveCfg(t)
	stats := processRemovals([]duplicateGroup{{hash: keep.hash, keep: keep, remove: []fileMeta{cand}}}, cfg)
	if stats.Skipped != 1 {
		t.Fatalf("stats = %+v, want skip replaced candidate", stats)
	}
	if _, err := os.Stat(cand.path); err != nil {
		t.Fatalf("replaced candidate should remain: %v", err)
	}
}

func TestRevalidateSymlinkCandidate(t *testing.T) {
	dir := t.TempDir()
	keep, cand := scanPair(t, dir, "a.txt", "b.txt", "payload")
	if err := os.Remove(cand.path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(keep.path, cand.path); err != nil {
		t.Fatal(err)
	}
	err := revalidatePair(keep, cand)
	if !errors.Is(err, errSymlink) && !errors.Is(err, errNotRegular) && !errors.Is(err, errReplaced) {
		t.Fatalf("err = %v, want symlink/not regular/replaced", err)
	}
}

func TestRevalidateSameInodeHardlink(t *testing.T) {
	dir := t.TempDir()
	keepPath := filepath.Join(dir, "keep.txt")
	linkPath := filepath.Join(dir, "link.txt")
	mustWrite(t, keepPath, "shared")
	if err := os.Link(keepPath, linkPath); err != nil {
		t.Skipf("hard links not supported: %v", err)
	}
	groups, err := findDuplicates(dir, 2, "first")
	if err != nil {
		t.Fatal(err)
	}
	// Two names, one inode: not distinct candidates, so no removable group.
	for _, g := range groups {
		if len(g.remove) != 0 {
			t.Fatalf("hard-linked names should not be planned for removal: %+v", g)
		}
	}

	// Even if a caller forces a pair, revalidation must skip.
	files, err := listFiles(dir, scanSkip{})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %d, want 2", len(files))
	}
	keep, cand := files[0], files[1]
	if err := revalidatePair(keep, cand); !errors.Is(err, errSameInode) && !errors.Is(err, errHardlink) {
		t.Fatalf("err = %v, want same inode or hard link", err)
	}
}

func TestRevalidateMutatedKeeperSkipsAll(t *testing.T) {
	dir := t.TempDir()
	keep, cand := scanPair(t, dir, "a.txt", "b.txt", "payload")
	// Keep mtime/size different by rewriting keeper.
	mustWrite(t, keep.path, "payload-changed")
	cfg := testRemoveCfg(t)
	stats := processRemovals([]duplicateGroup{{hash: keep.hash, keep: keep, remove: []fileMeta{cand}}}, cfg)
	if stats.Skipped != 1 || stats.Quarantined != 0 {
		t.Fatalf("stats = %+v, want skip because keeper changed", stats)
	}
	if _, err := os.Stat(cand.path); err != nil {
		t.Fatalf("candidate should remain: %v", err)
	}
}

func TestMtimeChangeIsModified(t *testing.T) {
	dir := t.TempDir()
	keep, cand := scanPair(t, dir, "a.txt", "b.txt", "payload")
	newTime := time.Now().Add(2 * time.Hour)
	if err := os.Chtimes(cand.path, newTime, newTime); err != nil {
		t.Fatal(err)
	}
	if err := revalidatePair(keep, cand); !errors.Is(err, errModified) {
		t.Fatalf("err = %v, want modified", err)
	}
}
