package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// Residual concurrent-mutation limit:
// Revalidation (type, identity, size, mtime, and byte-for-byte content)
// runs immediately before each destructive action, but it is not atomic
// with the following rename/trash. A replacement in that window can still
// race on every platform. This tool does not claim race-free deletion.
var (
	errMissing          = errors.New("missing")
	errModified         = errors.New("modified since scan")
	errReplaced         = errors.New("replaced since scan")
	errNotRegular       = errors.New("not a regular file")
	errSymlink          = errors.New("symbolic link")
	errSameInode        = errors.New("same inode as keeper")
	errHardlink         = errors.New("hard link (nlink > 1)")
	errContentMismatch  = errors.New("content does not match keeper")
	errNoIdentity       = errors.New("file identity unavailable")
	errKeeperUnreliable = errors.New("keeper failed revalidation")
	errSamePath         = errors.New("candidate path is the keeper")
)

type checkError struct {
	which  string // "keeper" or "candidate"
	path   string
	reason error
}

func (e *checkError) Error() string {
	return fmt.Sprintf("%s %s: %v", e.which, e.path, e.reason)
}

func (e *checkError) Unwrap() error { return e.reason }

type liveFile struct {
	path    string
	size    int64
	modTime time.Time
	mode    os.FileMode
	dev     uint64
	ino     uint64
	nlink   uint64
	idOK    bool
}

func lstatLive(path string) (liveFile, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return liveFile{path: path}, errMissing
		}
		return liveFile{path: path}, err
	}
	lf := liveFile{
		path:    path,
		size:    info.Size(),
		modTime: info.ModTime(),
		mode:    info.Mode(),
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return lf, errSymlink
	}
	if !info.Mode().IsRegular() {
		return lf, errNotRegular
	}
	if dev, ino, nlink, ok := identityFromInfo(info); ok {
		lf.dev, lf.ino, lf.nlink, lf.idOK = dev, ino, nlink, true
	}
	return lf, nil
}

func snapshotMatches(live liveFile, snap fileMeta) error {
	if live.size != snap.size || !live.modTime.Equal(snap.modTime) {
		return errModified
	}
	if snap.idOK && live.idOK {
		if live.dev != snap.dev || live.ino != snap.ino {
			return errReplaced
		}
	}
	return nil
}

// revalidatePair checks the keeper and candidate immediately before a
// destructive action. It compares each path against the scan snapshot and
// requires current byte-for-byte content equality (not just MD5).
func revalidatePair(keep, cand fileMeta) error {
	if keep.path == cand.path {
		return &checkError{which: "candidate", path: cand.path, reason: errSamePath}
	}

	klive, err := lstatLive(keep.path)
	if err != nil {
		return &checkError{which: "keeper", path: keep.path, reason: err}
	}
	if err := snapshotMatches(klive, keep); err != nil {
		return &checkError{which: "keeper", path: keep.path, reason: err}
	}

	clive, err := lstatLive(cand.path)
	if err != nil {
		return &checkError{which: "candidate", path: cand.path, reason: err}
	}
	if err := snapshotMatches(clive, cand); err != nil {
		return &checkError{which: "candidate", path: cand.path, reason: err}
	}

	if keep.idOK && cand.idOK && klive.idOK && clive.idOK {
		if klive.dev == clive.dev && klive.ino == clive.ino {
			return &checkError{which: "candidate", path: cand.path, reason: errSameInode}
		}
	}
	// Conservative: do not touch hard-linked names (nlink > 1).
	if clive.idOK && clive.nlink > 1 {
		return &checkError{which: "candidate", path: cand.path, reason: errHardlink}
	}
	if cand.idOK && cand.nlink > 1 {
		return &checkError{which: "candidate", path: cand.path, reason: errHardlink}
	}

	same, err := byteEqualNoFollow(keep.path, cand.path)
	if err != nil {
		return fmt.Errorf("compare %s and %s: %w", keep.path, cand.path, err)
	}
	if !same {
		return &checkError{which: "candidate", path: cand.path, reason: errContentMismatch}
	}

	// Re-lstat after the comparison to shrink (not close) the race window.
	klive2, err := lstatLive(keep.path)
	if err != nil {
		return &checkError{which: "keeper", path: keep.path, reason: err}
	}
	if err := snapshotMatches(klive2, keep); err != nil {
		return &checkError{which: "keeper", path: keep.path, reason: err}
	}
	if klive.idOK && klive2.idOK && (klive.dev != klive2.dev || klive.ino != klive2.ino) {
		return &checkError{which: "keeper", path: keep.path, reason: errReplaced}
	}
	clive2, err := lstatLive(cand.path)
	if err != nil {
		return &checkError{which: "candidate", path: cand.path, reason: err}
	}
	if err := snapshotMatches(clive2, cand); err != nil {
		return &checkError{which: "candidate", path: cand.path, reason: err}
	}
	if clive.idOK && clive2.idOK && (clive.dev != clive2.dev || clive.ino != clive2.ino) {
		return &checkError{which: "candidate", path: cand.path, reason: errReplaced}
	}
	return nil
}

func byteEqualNoFollow(pathA, pathB string) (bool, error) {
	fa, err := openNoFollow(pathA)
	if err != nil {
		return false, err
	}
	defer fa.Close()
	fb, err := openNoFollow(pathB)
	if err != nil {
		return false, err
	}
	defer fb.Close()

	if err := assertStillRegular(fa); err != nil {
		return false, err
	}
	if err := assertStillRegular(fb); err != nil {
		return false, err
	}

	ba := make([]byte, 32*1024)
	bb := make([]byte, 32*1024)
	for {
		na, ea := fa.Read(ba)
		nb, eb := fb.Read(bb)
		if na > 0 || nb > 0 {
			if na != nb || !bytes.Equal(ba[:na], bb[:nb]) {
				return false, nil
			}
		}
		if ea == io.EOF && eb == io.EOF {
			return true, nil
		}
		if ea != nil && ea != io.EOF {
			return false, ea
		}
		if eb != nil && eb != io.EOF {
			return false, eb
		}
		if ea == io.EOF || eb == io.EOF {
			return false, nil
		}
	}
}

func assertStillRegular(f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errSymlink
	}
	if !info.Mode().IsRegular() {
		return errNotRegular
	}
	return nil
}
