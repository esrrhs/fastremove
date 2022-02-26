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

	flag.Parse()

	if *path == "" || *method == "" {
		flag.Usage()
		return
	}

	total := 0
	filepath.Walk(*path, func(path string, info os.FileInfo, err error) error {

		if info == nil || info.IsDir() {
			return nil
		}

		total++
		return nil
	})

	fmt.Printf("total %v\n", total)

	var num int32
	cur := 0
	last := 0
	start := time.Now()
	filepath.Walk(*path, func(path string, info os.FileInfo, err error) error {

		if info == nil || info.IsDir() {
			return nil
		}

		if int(num) < *thread {
			atomic.AddInt32(&num, 1)
			go remove(path, &num, *method, *del, *v, &last, &cur, total, start)
		} else {
			atomic.AddInt32(&num, 1)
			remove(path, &num, *method, *del, *v, &last, &cur, total, start)
		}

		return nil
	})

	for num > 0 {
		time.Sleep(time.Millisecond * 10)
	}
}

var g_duplicate sync.Map
var g_duplicate_num int

func remove_duplicate(f string, del bool, show bool) {

	file, err := os.Open(f)
	if err != nil {
		panic(err)
	}

	hash := md5.New()
	_, err = io.Copy(hash, file)
	if err != nil {
		panic(err)
	}
	file.Close()

	m := hash.Sum(nil)
	mstr := fmt.Sprintf("%x", m)
	if show {
		fmt.Printf("done %v %v\n", f, mstr)
	}

	old, exist := g_duplicate.LoadOrStore(mstr, f)
	if exist {
		g_duplicate_num++
		fmt.Printf("duplicate %v %v %v, total %v\n", f, old.(string), mstr, g_duplicate_num)
		if del {
			err := os.Remove(f)
			if err != nil {
				panic(err)
			}
		}
	}
}

func remove(f string, num *int32, method string, del bool, show bool, last *int, cur *int, total int, start time.Time) {

	defer atomic.AddInt32(num, -1)

	if method == "duplicate" {
		remove_duplicate(f, del, show)
	} else {
		panic(fmt.Errorf("no method %v", method))
	}

	per := total / 100
	if per <= 0 {
		per = 1
	}
	*cur++
	step := *cur / per
	if step != *last {
		dur := time.Now().Sub(start) / time.Second
		speed := float64(*cur) / float64(dur)
		need := float64(total-*cur) / speed
		*last = step
		fmt.Printf("complete %v/100, left %vs\n", step, int(need))
	}

}
