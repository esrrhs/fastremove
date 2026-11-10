package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

func main() {
	path := flag.String("path", "./", "scan path")
	method := flag.String("method", "duplicate", "delete method: duplicate")
	del := flag.Bool("delete", false, "recoverably remove duplicates (trash if available, else quarantine); default: dry-run report only")
	thread := flag.Int("thread", 32, "hash worker concurrency")
	keep := flag.String("keep", "first", "keep strategy: first, newest, shortest-path")
	reportPath := flag.String("report", "", "optional path to write duplicate report (default: stdout)")
	quarantineDir := flag.String("quarantine-dir", "", "quarantine root used when trash is unavailable (default: user cache)")
	restore := flag.String("restore", "", "restore files from a quarantine manifest.json")
	v := flag.Bool("v", false, "show info")
	showVersion := flag.Bool("version", false, "show version")

	flag.Parse()

	if *showVersion {
		fmt.Println(Version)
		return
	}

	if *restore != "" {
		if err := restoreFromManifest(*restore, os.Stdout); err != nil {
			log.Fatalf("restore: %v", err)
		}
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

	skip := newQuarantineSkip(*quarantineDir)
	files, err := listFiles(*path, skip)
	if err != nil {
		log.Fatalf("list files: %v", err)
	}
	total := len(files)
	fmt.Printf("total %v\n", total)

	if total == 0 {
		fmt.Println("no files to scan")
		return
	}

	eligible, skippedUnique := sizeEligible(files)
	fmt.Printf("hashing %d file(s) (%d unique-size skipped)\n", len(eligible), skippedUnique)

	metas := hashFiles(eligible, *thread, *v)
	groups := groupDuplicates(metas, strategy)

	if err := reportWouldClobber(*reportPath, files, groups); err != nil {
		log.Fatal(err)
	}

	cfg := &removeConfig{
		Trash:         platformTrash(),
		QuarantineDir: *quarantineDir,
	}

	var stats *removalStats
	if *del {
		s := processRemovals(groups, cfg)
		stats = &s
	}

	reportWriter := io.Writer(os.Stdout)
	var reportFile *os.File
	if *reportPath != "" {
		reportFile, err = os.Create(*reportPath)
		if err != nil {
			log.Fatalf("create report %s: %v", *reportPath, err)
		}
		defer reportFile.Close()
		reportWriter = reportFile
	}

	nDupFiles, nGroups := writeReport(reportWriter, groups, *del, stats)
	if *reportPath != "" {
		fmt.Printf("report written to %s\n", *reportPath)
	}

	mode := "dry-run"
	if *del {
		mode = "delete (recoverable)"
	}
	fmt.Printf("duplicate groups: %d, removable files: %d, mode: %s, keep: %s\n",
		nGroups, nDupFiles, mode, strategy)

	if *del && stats != nil {
		fmt.Printf("trashed: %d, quarantined: %d, skipped: %d, failed: %d\n",
			stats.Trashed, stats.Quarantined, stats.Skipped, stats.Failed)
		if stats.ManifestPath != "" {
			fmt.Printf("quarantine manifest: %s\n", stats.ManifestPath)
			fmt.Printf("restore with: fastremove -restore %s\n", stats.ManifestPath)
		}
		if stats.Failed > 0 {
			os.Exit(1)
		}
	} else if nDupFiles > 0 {
		fmt.Println("dry-run: no files removed (pass -delete to trash or quarantine non-keepers)")
	}
}
