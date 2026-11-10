package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDryRunDoesNotDelete(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	mustWrite(t, a, "same")
	mustWrite(t, b, "same")

	groups, err := findDuplicates(dir, 4, "first")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	if len(groups[0].remove) != 1 {
		t.Fatalf("remove candidates = %d, want 1", len(groups[0].remove))
	}

	if _, err := os.Stat(a); err != nil {
		t.Fatalf("a should remain: %v", err)
	}
	if _, err := os.Stat(b); err != nil {
		t.Fatalf("b should remain without -delete: %v", err)
	}
}

func TestDeleteRemovesOnlyNonKeepers(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	mustWrite(t, a, "dup")
	mustWrite(t, b, "dup")

	groups, err := findDuplicates(dir, 4, "first")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}

	cfg := testRemoveCfg(t)
	stats := processRemovals(groups, cfg)
	if stats.Failed != 0 {
		t.Fatalf("failed = %d, results=%v", stats.Failed, stats.Results)
	}
	if stats.Quarantined != 1 {
		t.Fatalf("quarantined = %d, want 1 (stats=%+v)", stats.Quarantined, stats)
	}

	keepPath := groups[0].keep.path
	removePath := groups[0].remove[0].path
	if _, err := os.Stat(keepPath); err != nil {
		t.Fatalf("keeper %s should remain: %v", keepPath, err)
	}
	if _, err := os.Stat(removePath); !os.IsNotExist(err) {
		t.Fatalf("non-keeper %s should be removed, err=%v", removePath, err)
	}
}

func TestKeepFirstUsesWalkOrder(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	mustWrite(t, a, "x")
	mustWrite(t, b, "x")

	groups, err := findDuplicates(dir, 2, "first")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	if groups[0].keep.path != a {
		t.Fatalf("keep = %s, want first-seen %s", groups[0].keep.path, a)
	}
	if groups[0].remove[0].path != b {
		t.Fatalf("remove = %s, want %s", groups[0].remove[0].path, b)
	}
}

func TestKeepNewest(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.txt")
	newPath := filepath.Join(dir, "new.txt")
	mustWrite(t, oldPath, "payload")
	mustWrite(t, newPath, "payload")

	oldTime := time.Now().Add(-2 * time.Hour)
	newTime := time.Now().Add(-1 * time.Minute)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newPath, newTime, newTime); err != nil {
		t.Fatal(err)
	}

	groups, err := findDuplicates(dir, 2, "newest")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	if groups[0].keep.path != newPath {
		t.Fatalf("keep = %s, want newest %s", groups[0].keep.path, newPath)
	}
	if groups[0].remove[0].path != oldPath {
		t.Fatalf("remove = %s, want %s", groups[0].remove[0].path, oldPath)
	}
}

func TestKeepShortestPath(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "sub", "deep")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	shallow := filepath.Join(dir, "shallow.txt")
	deep := filepath.Join(nested, "deep.txt")
	mustWrite(t, shallow, "body")
	mustWrite(t, deep, "body")

	groups, err := findDuplicates(dir, 2, "shortest-path")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	if groups[0].keep.path != shallow {
		t.Fatalf("keep = %s, want shortest-path %s", groups[0].keep.path, shallow)
	}
	if groups[0].remove[0].path != deep {
		t.Fatalf("remove = %s, want %s", groups[0].remove[0].path, deep)
	}
}

func TestDifferentContentNotGrouped(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "one")
	mustWrite(t, filepath.Join(dir, "b.txt"), "two")

	groups, err := findDuplicates(dir, 2, "first")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("groups = %d, want 0", len(groups))
	}
}

func TestPickKeeperStrategiesUnit(t *testing.T) {
	now := time.Now()
	members := []fileMeta{
		{path: "/a/b/c.txt", order: 2, modTime: now.Add(-time.Hour)},
		{path: "/z.txt", order: 0, modTime: now},
		{path: "/a/x.txt", order: 1, modTime: now.Add(-time.Minute)},
	}

	if idx := pickKeeper(members, "first"); members[idx].path != "/z.txt" {
		t.Fatalf("first keep = %s, want /z.txt", members[idx].path)
	}
	if idx := pickKeeper(members, "newest"); members[idx].path != "/z.txt" {
		t.Fatalf("newest keep = %s, want /z.txt", members[idx].path)
	}
	if idx := pickKeeper(members, "shortest-path"); members[idx].path != "/z.txt" {
		t.Fatalf("shortest-path keep = %s, want /z.txt", members[idx].path)
	}

	tied := []fileMeta{
		{path: "/bb.txt", order: 0, modTime: now},
		{path: "/aa.txt", order: 1, modTime: now},
	}
	if idx := pickKeeper(tied, "shortest-path"); tied[idx].path != "/aa.txt" {
		t.Fatalf("shortest-path lexical keep = %s, want /aa.txt", tied[idx].path)
	}
}

func TestVersionNotEmpty(t *testing.T) {
	if Version == "" {
		t.Fatal("Version should not be empty")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testRemoveCfg(t *testing.T) *removeConfig {
	t.Helper()
	return &removeConfig{
		Trash:         unsupportedTrash{},
		QuarantineDir: filepath.Join(t.TempDir(), "quarantine"),
	}
}
