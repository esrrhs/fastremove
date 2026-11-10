package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	quarantineDirName    = ".fastremove-quarantine"
	quarantineMarkerName = ".fastremove-quarantine-root"
)

type fileMeta struct {
	path    string
	hash    string
	size    int64
	modTime time.Time
	order   int // walk discovery order for "first" strategy
	dev     uint64
	ino     uint64
	nlink   uint64
	idOK    bool
}

type skipItem struct {
	file   fileMeta
	reason string
}

type duplicateGroup struct {
	hash   string
	keep   fileMeta
	remove []fileMeta
	skip   []skipItem
}

type scanSkip struct {
	absRoots []string
}

func (s scanSkip) shouldSkipDir(path string) bool {
	base := filepath.Base(path)
	if base == quarantineDirName {
		return true
	}
	if markerExists(path) {
		return true
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for _, root := range s.absRoots {
		if root == "" {
			continue
		}
		if abs == root || isUnder(abs, root) {
			return true
		}
	}
	return false
}

func markerExists(dir string) bool {
	info, err := os.Lstat(filepath.Join(dir, quarantineMarkerName))
	return err == nil && info.Mode().IsRegular()
}

func isUnder(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return true
}

func listFiles(root string, skip scanSkip) ([]fileMeta, error) {
	var files []fileMeta
	order := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			log.Printf("walk error %s: %v", path, err)
			return nil
		}
		if d.IsDir() {
			if path != root && skip.shouldSkipDir(path) {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			log.Printf("stat %s: %v", path, err)
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil
		}
		m := fileMeta{
			path:    path,
			size:    info.Size(),
			modTime: info.ModTime(),
			order:   order,
		}
		if dev, ino, nlink, ok := identityFromInfo(info); ok {
			m.dev, m.ino, m.nlink, m.idOK = dev, ino, nlink, true
		}
		files = append(files, m)
		order++
		return nil
	})
	return files, err
}

func uniqueIDKey(m fileMeta) string {
	if m.idOK {
		return fmt.Sprintf("%d:%d", m.dev, m.ino)
	}
	return "path:" + m.path
}

func sizeEligible(files []fileMeta) (eligible []fileMeta, skippedUnique int) {
	bySize := make(map[int64][]fileMeta)
	for _, f := range files {
		bySize[f.size] = append(bySize[f.size], f)
	}
	for _, group := range bySize {
		seen := make(map[string]struct{})
		for _, f := range group {
			seen[uniqueIDKey(f)] = struct{}{}
		}
		if len(seen) < 2 {
			skippedUnique += len(group)
			continue
		}
		eligible = append(eligible, group...)
	}
	return eligible, skippedUnique
}

func hashFiles(files []fileMeta, workers int, verbose bool) []fileMeta {
	type result struct {
		meta fileMeta
		ok   bool
	}

	jobs := make(chan fileMeta)
	results := make(chan result)
	var wg sync.WaitGroup

	var (
		cur   int32
		last  int32
		start = time.Now()
		total = len(files)
	)

	if workers < 1 {
		workers = 1
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range jobs {
				hashed, err := hashOne(f)
				if err != nil {
					log.Printf("hash %s: %v", f.path, err)
					results <- result{ok: false}
					continue
				}
				if verbose {
					fmt.Printf("done %v %v\n", hashed.path, hashed.hash)
				}
				results <- result{meta: hashed, ok: true}
			}
		}()
	}

	go func() {
		for _, f := range files {
			jobs <- f
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	out := make([]fileMeta, 0, len(files))
	for r := range results {
		c := int(atomic.AddInt32(&cur, 1))
		reportProgress(c, total, &last, start)
		if r.ok {
			out = append(out, r.meta)
		}
	}
	return out
}

// testOnHash is an optional test seam invoked once per hashed path.
var testOnHash func(path string)

func hashOne(f fileMeta) (fileMeta, error) {
	if testOnHash != nil {
		testOnHash(f.path)
	}
	file, err := openNoFollow(f.path)
	if err != nil {
		return f, err
	}
	defer file.Close()

	dev, ino, nlink, size, mtime, mode, err := identityFromFile(file)
	if err != nil {
		return f, err
	}
	if mode&os.ModeSymlink != 0 {
		return f, errSymlink
	}
	if !mode.IsRegular() {
		return f, errNotRegular
	}
	// File changed between list and open: do not use an unstable snapshot.
	if size != f.size {
		return f, errModified
	}
	if f.idOK && dev != f.dev || f.idOK && ino != f.ino {
		return f, errReplaced
	}

	h := md5.New()
	if _, err = io.Copy(h, file); err != nil {
		return f, err
	}
	f.hash = hex.EncodeToString(h.Sum(nil))
	f.size = size
	f.modTime = mtime
	if dev != 0 || ino != 0 || nlink != 0 {
		f.dev, f.ino, f.nlink, f.idOK = dev, ino, nlink, true
	}
	return f, nil
}

func reportProgress(c, total int, last *int32, start time.Time) {
	if total <= 0 {
		return
	}
	per := total / 100
	if per <= 0 {
		per = 1
	}
	step := int32(c / per)
	for {
		prev := atomic.LoadInt32(last)
		if step <= prev {
			return
		}
		if atomic.CompareAndSwapInt32(last, prev, step) {
			break
		}
	}
	dur := time.Since(start).Seconds()
	if dur <= 0 {
		fmt.Printf("complete %v/100, left ?s\n", step)
		return
	}
	speed := float64(c) / dur
	need := float64(total-c) / speed
	fmt.Printf("complete %v/100, left %vs\n", step, int(need))
}

func groupDuplicates(metas []fileMeta, strategy string) []duplicateGroup {
	byHash := make(map[string][]fileMeta)
	for _, m := range metas {
		if m.hash == "" {
			continue
		}
		byHash[m.hash] = append(byHash[m.hash], m)
	}

	var groups []duplicateGroup
	for hash, members := range byHash {
		if len(members) < 2 {
			continue
		}
		keepIdx := pickKeeper(members, strategy)
		keep := members[keepIdx]
		var remove []fileMeta
		var skipped []skipItem
		for i, m := range members {
			if i == keepIdx {
				continue
			}
			if reason := planSkipReason(keep, m); reason != "" {
				skipped = append(skipped, skipItem{file: m, reason: reason})
				continue
			}
			remove = append(remove, m)
		}
		if len(remove) == 0 && len(skipped) == 0 {
			continue
		}
		sort.Slice(remove, func(i, j int) bool {
			return remove[i].path < remove[j].path
		})
		sort.Slice(skipped, func(i, j int) bool {
			return skipped[i].file.path < skipped[j].file.path
		})
		groups = append(groups, duplicateGroup{hash: hash, keep: keep, remove: remove, skip: skipped})
	}

	sort.Slice(groups, func(i, j int) bool {
		if groups[i].hash != groups[j].hash {
			return groups[i].hash < groups[j].hash
		}
		return groups[i].keep.path < groups[j].keep.path
	})
	return groups
}

func planSkipReason(keep, cand fileMeta) string {
	if keep.path == cand.path {
		return errSamePath.Error()
	}
	if keep.idOK && cand.idOK && keep.dev == cand.dev && keep.ino == cand.ino {
		return errSameInode.Error()
	}
	if cand.idOK && cand.nlink > 1 {
		return errHardlink.Error()
	}
	return ""
}

func pickKeeper(members []fileMeta, strategy string) int {
	best := 0
	for i := 1; i < len(members); i++ {
		if betterKeeper(members[i], members[best], strategy) {
			best = i
		}
	}
	return best
}

func betterKeeper(a, b fileMeta, strategy string) bool {
	switch strategy {
	case "newest":
		if !a.modTime.Equal(b.modTime) {
			return a.modTime.After(b.modTime)
		}
		if a.order != b.order {
			return a.order < b.order
		}
		return a.path < b.path
	case "shortest-path":
		sa, sb := pathSepCount(a.path), pathSepCount(b.path)
		if sa != sb {
			return sa < sb
		}
		if len(a.path) != len(b.path) {
			return len(a.path) < len(b.path)
		}
		return a.path < b.path
	default: // first
		if a.order != b.order {
			return a.order < b.order
		}
		return a.path < b.path
	}
}

func pathSepCount(p string) int {
	n := 0
	for _, r := range p {
		if r == '/' || r == '\\' {
			n++
		}
	}
	return n
}

// findDuplicates is the testable core: scan root, hash eligible size groups, group by strategy.
func findDuplicates(root string, workers int, strategy string) ([]duplicateGroup, error) {
	return findDuplicatesSkip(root, workers, strategy, scanSkip{})
}

func findDuplicatesSkip(root string, workers int, strategy string, skip scanSkip) ([]duplicateGroup, error) {
	files, err := listFiles(root, skip)
	if err != nil {
		return nil, err
	}
	eligible, _ := sizeEligible(files)
	metas := hashFiles(eligible, workers, false)
	return groupDuplicates(metas, strategy), nil
}
