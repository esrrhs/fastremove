//go:build !linux && !darwin && !windows

package main

func platformTrash() trashBackend {
	return unsupportedTrash{}
}
