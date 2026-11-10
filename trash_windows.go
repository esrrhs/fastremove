//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"
)

const (
	foDelete          = 0x0003
	fofSilent         = 0x0004
	fofNoConfirmation = 0x0010
	fofAllowUndo      = 0x0040
	fofNoErrorUI      = 0x0400
	fofNoConfirmMkdir = 0x0200
)

type shFileOpStruct struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

type windowsRecycleTrash struct{}

func (windowsRecycleTrash) Name() string    { return "recycle-bin" }
func (windowsRecycleTrash) Available() bool { return true }

func (windowsRecycleTrash) Trash(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	from, err := syscall.UTF16PtrFromString(abs + "\x00")
	if err != nil {
		return "", err
	}
	op := shFileOpStruct{
		wFunc:  foDelete,
		pFrom:  from,
		fFlags: fofAllowUndo | fofNoConfirmation | fofNoErrorUI | fofSilent | fofNoConfirmMkdir,
	}
	r, _, callErr := syscall.NewLazyDLL("shell32.dll").NewProc("SHFileOperationW").Call(uintptr(unsafe.Pointer(&op)))
	if r != 0 {
		if callErr != nil && callErr != syscall.Errno(0) {
			return "", fmt.Errorf("recycle bin: %w", callErr)
		}
		return "", fmt.Errorf("recycle bin: SHFileOperationW status %d", r)
	}
	if op.fAnyOperationsAborted != 0 {
		return "", fmt.Errorf("recycle bin: operation aborted")
	}
	return "Recycle Bin", nil
}

func platformTrash() trashBackend {
	return windowsRecycleTrash{}
}
