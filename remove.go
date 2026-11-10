package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type removeConfig struct {
	Trash         trashBackend
	QuarantineDir string // root; a session subdirectory is created
	session       *quarantineSession
}

type quarantineSession struct {
	dir      string
	filesDir string
	manifest string
	data     recoveryManifest
}

type recoveryManifest struct {
	Version int            `json:"version"`
	Created string         `json:"created"`
	Dir     string         `json:"dir"`
	Items   []recoveryItem `json:"items"`
}

type recoveryItem struct {
	Original   string `json:"original"`
	Quarantine string `json:"quarantine"`
	Size       int64  `json:"size"`
	ModTime    string `json:"modTime"`
	Hash       string `json:"hash,omitempty"`
	Restored   bool   `json:"restored,omitempty"`
}

type actionResult struct {
	Path     string
	Action   string // trashed, quarantined, skipped, failed, would-trash-or-quarantine
	Reason   string
	Recovery string
}

type removalStats struct {
	Trashed      int
	Quarantined  int
	Skipped      int
	Failed       int
	Results      []actionResult
	RecoveryDir  string
	ManifestPath string
}

func defaultQuarantineRoot() (string, error) {
	if v := os.Getenv("FASTREMOVE_QUARANTINE"); v != "" {
		return v, nil
	}
	cache, err := os.UserCacheDir()
	if err == nil && cache != "" {
		return filepath.Join(cache, "fastremove", "quarantine"), nil
	}
	return filepath.Join(os.TempDir(), "fastremove-quarantine"), nil
}

func newQuarantineSkip(extra ...string) scanSkip {
	var roots []string
	if r, err := defaultQuarantineRoot(); err == nil {
		if abs, err := filepath.Abs(r); err == nil {
			roots = append(roots, abs)
		}
	}
	for _, e := range extra {
		if e == "" {
			continue
		}
		if abs, err := filepath.Abs(e); err == nil {
			roots = append(roots, abs)
		}
	}
	return scanSkip{absRoots: roots}
}

func processRemovals(groups []duplicateGroup, cfg *removeConfig) removalStats {
	var stats removalStats
	for _, g := range groups {
		for _, s := range g.skip {
			stats.Skipped++
			stats.Results = append(stats.Results, actionResult{
				Path:   s.file.path,
				Action: "skipped",
				Reason: s.reason,
			})
		}
		for _, cand := range g.remove {
			if err := revalidatePair(g.keep, cand); err != nil {
				stats.Skipped++
				stats.Results = append(stats.Results, actionResult{
					Path:   cand.path,
					Action: "skipped",
					Reason: err.Error(),
				})
				continue
			}
			res := removeOne(cand, cfg)
			stats.Results = append(stats.Results, res)
			switch res.Action {
			case "trashed":
				stats.Trashed++
			case "quarantined":
				stats.Quarantined++
			case "skipped":
				stats.Skipped++
			default:
				stats.Failed++
			}
		}
	}
	if cfg != nil && cfg.session != nil {
		stats.RecoveryDir = cfg.session.dir
		stats.ManifestPath = cfg.session.manifest
	}
	return stats
}

func removeOne(cand fileMeta, cfg *removeConfig) actionResult {
	if cfg != nil && cfg.Trash != nil && cfg.Trash.Available() {
		rec, err := cfg.Trash.Trash(cand.path)
		if err == nil {
			return actionResult{Path: cand.path, Action: "trashed", Recovery: rec}
		}
		log.Printf("trash %s (%s): %v; falling back to quarantine", cand.path, cfg.Trash.Name(), err)
	}
	rec, err := quarantineMove(cand, cfg)
	if err != nil {
		return actionResult{Path: cand.path, Action: "failed", Reason: err.Error(), Recovery: rec}
	}
	return actionResult{Path: cand.path, Action: "quarantined", Recovery: rec}
}

func (cfg *removeConfig) ensureSession() error {
	if cfg.session != nil {
		return nil
	}
	root := cfg.QuarantineDir
	if root == "" {
		var err error
		root, err = defaultQuarantineRoot()
		if err != nil {
			return err
		}
		cfg.QuarantineDir = root
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	if err := writeMarker(root); err != nil {
		return err
	}
	sessionName := time.Now().Format("20060102T150405.000000000")
	dir := filepath.Join(root, sessionName)
	if err := os.MkdirAll(filepath.Join(dir, "files"), 0o700); err != nil {
		return err
	}
	if err := writeMarker(dir); err != nil {
		return err
	}
	cfg.session = &quarantineSession{
		dir:      dir,
		filesDir: filepath.Join(dir, "files"),
		manifest: filepath.Join(dir, "manifest.json"),
		data: recoveryManifest{
			Version: 1,
			Created: time.Now().UTC().Format(time.RFC3339Nano),
			Dir:     dir,
		},
	}
	return cfg.session.flushManifest()
}

func writeMarker(dir string) error {
	p := filepath.Join(dir, quarantineMarkerName)
	if _, err := os.Lstat(p); err == nil {
		return nil
	}
	return os.WriteFile(p, []byte("fastremove quarantine root — do not edit\n"), 0o644)
}

func quarantineMove(cand fileMeta, cfg *removeConfig) (string, error) {
	if cfg == nil {
		return "", errors.New("no quarantine configuration")
	}
	if err := cfg.ensureSession(); err != nil {
		return "", err
	}
	s := cfg.session
	abs, err := filepath.Abs(cand.path)
	if err != nil {
		return "", err
	}
	dest, err := uniqueQuarantineName(s.filesDir, filepath.Base(abs))
	if err != nil {
		return "", err
	}

	// Sidecar mapping written first so a crash mid-rename is still recoverable.
	side := dest + ".origin.json"
	item := recoveryItem{
		Original:   abs,
		Quarantine: dest,
		Size:       cand.size,
		ModTime:    cand.modTime.Format(time.RFC3339Nano),
		Hash:       cand.hash,
	}
	if err := writeJSONAtomic(side, item); err != nil {
		return "", fmt.Errorf("write origin sidecar: %w", err)
	}

	if err := os.Rename(abs, dest); err != nil {
		_ = os.Remove(side)
		if isCrossDevice(err) {
			return "", fmt.Errorf("quarantine is on a different device; refusing copy-delete: %w", err)
		}
		return "", err
	}

	s.data.Items = append(s.data.Items, item)
	if err := s.flushManifest(); err != nil {
		// Fail closed: try to put the file back. Keep the sidecar if that fails.
		if backErr := os.Rename(dest, abs); backErr != nil {
			return dest, fmt.Errorf("manifest write failed (%v) and rollback failed (%v); file is at %s", err, backErr, dest)
		}
		_ = os.Remove(side)
		s.data.Items = s.data.Items[:len(s.data.Items)-1]
		return "", fmt.Errorf("manifest write failed, file restored: %w", err)
	}
	return dest, nil
}

func uniqueQuarantineName(dir, base string) (string, error) {
	if base == "" || base == "." || base == ".." {
		base = "file"
	}
	base = filepath.Base(base)
	for i := 1; i < 100000; i++ {
		name := fmt.Sprintf("%06d-%s", i, base)
		dest := filepath.Join(dir, name)
		if _, err := os.Lstat(dest); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return "", err
		}
		if _, err := os.Lstat(dest + ".origin.json"); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return "", err
		}
		return dest, nil
	}
	return "", fmt.Errorf("too many quarantine name collisions for %s", base)
}

// testForceManifestErr, if set, is returned by flushManifest after the
// session already has items (used to test fail-closed rollback).
var testForceManifestErr error

func (s *quarantineSession) flushManifest() error {
	if testForceManifestErr != nil && len(s.data.Items) > 0 {
		return testForceManifestErr
	}
	return writeJSONAtomic(s.manifest, s.data)
}

func writeJSONAtomic(path string, v any) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-manifest-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	ok := false
	defer func() {
		if !ok {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()
	if err := enc.Encode(v); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	ok = true
	return nil
}

func reportWouldClobber(reportPath string, files []fileMeta, groups []duplicateGroup) error {
	if reportPath == "" {
		return nil
	}
	abs, err := filepath.Abs(reportPath)
	if err != nil {
		return err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	// Never overwrite a symlink or non-regular file in the scan tree.
	for _, f := range files {
		fabs, err := filepath.Abs(f.path)
		if err != nil {
			continue
		}
		if fabs == abs {
			return fmt.Errorf("refusing to write report over scanned file %s", abs)
		}
		if sameExistingFile(abs, info, f.path) {
			return fmt.Errorf("refusing to write report over scanned file %s", f.path)
		}
	}
	for _, g := range groups {
		for _, p := range append([]string{g.keep.path}, pathsOf(g.remove)...) {
			if sameExistingFile(abs, info, p) {
				return fmt.Errorf("refusing to write report over scanned file %s", p)
			}
		}
	}
	return nil
}

func pathsOf(ms []fileMeta) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.path
	}
	return out
}

func sameExistingFile(reportAbs string, reportInfo os.FileInfo, other string) bool {
	oabs, err := filepath.Abs(other)
	if err != nil {
		return false
	}
	if oabs == reportAbs {
		return true
	}
	oinfo, err := os.Lstat(oabs)
	if err != nil {
		return false
	}
	return os.SameFile(reportInfo, oinfo)
}

func restoreFromManifest(manifestPath string, w io.Writer) error {
	abs, err := filepath.Abs(manifestPath)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	var man recoveryManifest
	if err := json.Unmarshal(data, &man); err != nil {
		return fmt.Errorf("invalid manifest: %w", err)
	}
	if man.Version != 1 {
		return fmt.Errorf("unsupported manifest version %d", man.Version)
	}
	var restored, skipped, failed int
	for i, item := range man.Items {
		if item.Restored {
			skipped++
			fmt.Fprintf(w, "skipped: %s (already restored)\n", item.Original)
			continue
		}
		if err := restoreOne(item); err != nil {
			failed++
			fmt.Fprintf(w, "failed: %s (%v)\n", item.Original, err)
			continue
		}
		man.Items[i].Restored = true
		restored++
		fmt.Fprintf(w, "restored: %s\n", item.Original)
	}
	if err := writeJSONAtomic(abs, man); err != nil {
		return fmt.Errorf("update manifest after restore: %w", err)
	}
	fmt.Fprintf(w, "restore complete: restored %d, skipped %d, failed %d\n", restored, skipped, failed)
	if failed > 0 {
		return fmt.Errorf("%d restore operation(s) failed", failed)
	}
	return nil
}

func restoreOne(item recoveryItem) error {
	orig, err := trustedRestorePath(item.Original)
	if err != nil {
		return err
	}
	src, err := trustedQuarantinePath(item.Quarantine)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(orig); err == nil {
		return fmt.Errorf("collision: destination already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(orig), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, orig); err != nil {
		if isCrossDevice(err) {
			return fmt.Errorf("cross-device restore refused (no copy): %w", err)
		}
		return err
	}
	_ = os.Remove(src + ".origin.json")
	return nil
}

func trustedRestorePath(p string) (string, error) {
	if p == "" {
		return "", errors.New("empty original path")
	}
	if strings.Contains(p, "..") {
		return "", errors.New("refusing path that contains ..")
	}
	if !filepath.IsAbs(p) {
		return "", errors.New("refusing non-absolute original path")
	}
	clean := filepath.Clean(p)
	if !filepath.IsAbs(clean) {
		return "", errors.New("refusing non-absolute original path")
	}
	if strings.Contains(clean, "..") {
		return "", errors.New("refusing path that still contains ..")
	}
	// Refuse if any existing ancestor is a symlink (path traversal).
	if err := refuseSymlinkAncestors(clean); err != nil {
		return "", err
	}
	return clean, nil
}

func trustedQuarantinePath(p string) (string, error) {
	if p == "" || !filepath.IsAbs(p) {
		return "", errors.New("refusing non-absolute quarantine path")
	}
	clean := filepath.Clean(p)
	info, err := os.Lstat(clean)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", errNotRegular
	}
	if err := refuseSymlinkAncestors(clean); err != nil {
		return "", err
	}
	return clean, nil
}

func refuseSymlinkAncestors(path string) error {
	dir := filepath.Dir(path)
	for {
		info, err := os.Lstat(dir)
		if err != nil {
			if os.IsNotExist(err) {
				// Parent will be created; keep walking up.
			} else {
				return err
			}
		} else if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing path with symlink ancestor %s", dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}

func writeReport(w io.Writer, groups []duplicateGroup, del bool, stats *removalStats) (dupFiles, nGroups int) {
	nGroups = len(groups)
	byPath := map[string]actionResult{}
	if stats != nil {
		for _, r := range stats.Results {
			byPath[r.Path] = r
		}
	}
	for _, g := range groups {
		dupFiles += len(g.remove)
		fmt.Fprintf(w, "group hash=%s files=%d\n", g.hash, 1+len(g.remove)+len(g.skip))
		fmt.Fprintf(w, "  keep: %s\n", g.keep.path)
		for _, s := range g.skip {
			fmt.Fprintf(w, "  skipped: %s (%s)\n", s.file.path, s.reason)
		}
		for _, r := range g.remove {
			if res, ok := byPath[r.path]; ok {
				writeActionLine(w, res)
				continue
			}
			if del {
				fmt.Fprintf(w, "  planned: %s (trash if available, else quarantine)\n", r.path)
			} else {
				fmt.Fprintf(w, "  would remove (trash/quarantine): %s\n", r.path)
			}
		}
	}
	return dupFiles, nGroups
}

func writeActionLine(w io.Writer, res actionResult) {
	switch res.Action {
	case "trashed":
		fmt.Fprintf(w, "  trashed: %s -> %s\n", res.Path, res.Recovery)
	case "quarantined":
		fmt.Fprintf(w, "  quarantined: %s -> %s\n", res.Path, res.Recovery)
	case "skipped":
		fmt.Fprintf(w, "  skipped: %s (%s)\n", res.Path, res.Reason)
	default:
		fmt.Fprintf(w, "  failed: %s (%s)\n", res.Path, res.Reason)
		if res.Recovery != "" {
			fmt.Fprintf(w, "    recovery: %s\n", res.Recovery)
		}
	}
}
