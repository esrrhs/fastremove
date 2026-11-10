//go:build linux

package main

func platformTrash() trashBackend {
	root := defaultFreedesktopTrashRoot()
	if root == "" {
		return unsupportedTrash{}
	}
	return freedesktopTrash{root: root}
}
