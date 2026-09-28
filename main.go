package main

import (
	"crypto/md5"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	path := flag.String("path", "./", "scan path")
	method := flag.String("method", "duplicate", "delete method: duplicate")
	del := flag.Bool("delete", false, "auto delete")
	thread := flag.Int("thread", 32, "replace thread")
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

	if *thread < 1 {
		*thread = 1
	}

	total := 0
	_ = filepath.Walk(*path, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		total++
		return nil
	})

	fmt.Printf("total %v\n", total)

	var (
		cur   int32
		last  int32
		start = time.Now()
		sem   = make(chan struct{}, *thread)
		wg    sync.WaitGroup
	)

	_ = filepath.Walk(*path, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(path string) {
			defer wg.Done()
			defer func() { <-sem }()
			remove(path, *method, *del, *v, &last, &cur, total, start)
		}(path)

		return nil
	})

	wg.Wait()
}

var (
	gDuplicate    sync.Map
	gDuplicateNum int32
)

func removeDuplicate(f string, del bool, show bool) {
	file, err := os.Open(f)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	hash := md5.New()
	if _, err = io.Copy(hash, file); err != nil {
		panic(err)
	}

	mstr := fmt.Sprintf("%x", hash.Sum(nil))
	if show {
		fmt.Printf("done %v %v\n", f, mstr)
	}

	old, exist := gDuplicate.LoadOrStore(mstr, f)
	if !exist {
		return
	}

	n := atomic.AddInt32(&gDuplicateNum, 1)
	fmt.Printf("duplicate %v %v %v, total %v\n", f, old.(string), mstr, n)
	if del {
		if err := os.Remove(f); err != nil {
			panic(err)
		}
	}
}

func remove(f string, method string, del bool, show bool, last *int32, cur *int32, total int, start time.Time) {
	if method == "duplicate" {
		removeDuplicate(f, del, show)
	} else {
		panic(fmt.Errorf("no method %v", method))
	}

	per := total / 100
	if per <= 0 {
		per = 1
	}
	c := int(atomic.AddInt32(cur, 1))
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
