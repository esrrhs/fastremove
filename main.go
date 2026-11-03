package main

import (
	"crypto/md5"
	"encoding/hex"
	"flag"
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

type fileMeta struct {
	path    string
	hash    string
	size    int64
	modTime time.Time
	order   int // walk discovery order for "first" strategy
}

type duplicateGroup struct {
	hash   string
	keep   fileMeta
	remove []fileMeta
}

func main() {
	path := flag.String("path", "./", "scan path")
	method := flag.String("method", "duplicate", "delete method: duplicate")
	del := flag.Bool("delete", false, "actually delete duplicate files (default: dry-run report only)")
	thread := flag.Int("thread", 32, "hash worker concurrency")
	keep := flag.String("keep", "first", "keep strategy: first, newest, shortest-path")
	reportPath := flag.String("report", "", "optional path to write duplicate report (default: stdout)")
	v := flag.Bool("v", false, "show info")
	showVersion := flag.Bool("version", false, "show version")

	flag.Parse()

	if *showVersion {
		fmt.Println(Version)
		return
	}

	if *path == "" || *method == "" {
		flag.Usage()
		return
	}

	if *method != "duplicate" {
		log.Fatalf("unsupported method %q (only \"duplicate\" is supported)", *method)
	}

	strategy := strings.ToLower(strings.TrimSpace(*keep))
	switch strategy {
	case "first", "newest", "shortest-path":
	default:
		log.Fatalf("unsupported keep strategy %q (use first, newest, or shortest-path)", *keep)
	}

	if *thread < 1 {
		*thread = 1
	}

	files, err := listFiles(*path)
	if err != nil {
		log.Fatalf("list files: %v", err)
	}
	total := len(files)
	fmt.Printf("total %v\n", total)

	if total == 0 {
		fmt.Println("no files to scan")
		return
	}

	metas := hashFiles(files, *thread, *v)
	groups := groupDuplicates(metas, strategy)

	reportWriter := os.Stdout
	var reportFile *os.File
	if *reportPath != "" {
		reportFile, err = os.Create(*reportPath)
		if err != nil {
			log.Fatalf("create report %s: %v", *reportPath, err)
		}
		defer reportFile.Close()
		reportWriter = reportFile
	}

	nDupFiles, nGroups := writeReport(reportWriter, groups, *del)
	if *reportPath != "" {
		fmt.Printf("report written to %s\n", *reportPath)
	}
	fmt.Printf("duplicate groups: %d, duplicate files: %d, mode: %s, keep: %s\n",
		nGroups, nDupFiles, modeLabel(*del), strategy)

	if *del {
		deleted, failed := deleteDuplicates(groups)
		fmt.Printf("deleted %d file(s)", deleted)
		if failed > 0 {
			fmt.Printf(", %d failed", failed)
		}
		fmt.Println()
	} else if nDupFiles > 0 {
		fmt.Println("dry-run: no files deleted (pass -delete to remove non-keepers)")
	}
}

func modeLabel(del bool) string {
	if del {
		return "delete"
	}
	return "dry-run"
}

func listFiles(root string) ([]fileMeta, error) {
	var files []fileMeta
	order := 0
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("walk error %s: %v", path, err)
			return nil
		}
		if info == nil || info.IsDir() {
			return nil
		}
		files = append(files, fileMeta{
			path:    path,
			size:    info.Size(),
			modTime: info.ModTime(),
			order:   order,
		})
		order++
		return nil
	})
	return files, err
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

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range jobs {
				hash, err := hashFile(f.path)
				if err != nil {
					log.Printf("hash %s: %v", f.path, err)
					results <- result{ok: false}
					continue
				}
				f.hash = hash
				if verbose {
					fmt.Printf("done %v %v\n", f.path, hash)
				}
				results <- result{meta: f, ok: true}
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

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	h := md5.New()
	if _, err = io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func reportProgress(c, total int, last *int32, start time.Time) {
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
		for i, m := range members {
			if i == keepIdx {
				continue
			}
			remove = append(remove, m)
		}
		// Stable-ish output: sort remove candidates by path
		sort.Slice(remove, func(i, j int) bool {
			return remove[i].path < remove[j].path
		})
		groups = append(groups, duplicateGroup{hash: hash, keep: keep, remove: remove})
	}

	sort.Slice(groups, func(i, j int) bool {
		if groups[i].hash != groups[j].hash {
			return groups[i].hash < groups[j].hash
		}
		return groups[i].keep.path < groups[j].keep.path
	})
	return groups
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
		// tie-break: earlier walk order, then lexical path
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

func writeReport(w io.Writer, groups []duplicateGroup, del bool) (dupFiles, nGroups int) {
	nGroups = len(groups)
	action := "would delete"
	if del {
		action = "delete"
	}
	for _, g := range groups {
		dupFiles += len(g.remove)
		fmt.Fprintf(w, "group hash=%s files=%d\n", g.hash, 1+len(g.remove))
		fmt.Fprintf(w, "  keep: %s\n", g.keep.path)
		for _, r := range g.remove {
			fmt.Fprintf(w, "  %s: %s\n", action, r.path)
		}
	}
	return dupFiles, nGroups
}

func deleteDuplicates(groups []duplicateGroup) (deleted, failed int) {
	for _, g := range groups {
		for _, r := range g.remove {
			if err := os.Remove(r.path); err != nil {
				log.Printf("remove %s: %v", r.path, err)
				failed++
				continue
			}
			deleted++
		}
	}
	return deleted, failed
}

// findDuplicates is the testable core: scan root, hash with workers, group by strategy.
func findDuplicates(root string, workers int, strategy string) ([]duplicateGroup, error) {
	files, err := listFiles(root)
	if err != nil {
		return nil, err
	}
	metas := hashFiles(files, workers, false)
	return groupDuplicates(metas, strategy), nil
}
