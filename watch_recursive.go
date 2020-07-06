package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/fsnotify/fsnotify"
)

var watcher *fsnotify.Watcher

// Op describes a set of file operations.
type operation uint32

// These are the generalized file operations that can trigger a notification.
const (
	copy operation = 1 << iota
	remove
)

type action struct {
	path   string
	action operation
}

var fileOperations chan action

func handleFile() {
	for {
		select {
		case file := <-fileOperations:
			fmt.Printf("EVENT! %#v\n", file)
		}
	}
}

func determineAction(path string) {
	op := copy
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		op = remove
	}

	if op == remove || !fi.IsDir() {
		fileOperations <- action{path, op}
	} else if err := filepath.Walk(path, watchDir); err != nil {
		fmt.Println("ERROR", err)
	}
}

// watchDir gets run as a walk func, searching for directories to add watchers to
func watchDir(path string, fi os.FileInfo, err error) error {

	// since fsnotify can watch all the files in a directory, watchers only need
	// to be added to each nested directory
	if fi.Mode().IsDir() {
		return watcher.Add(path)
	}
	determineAction(path)

	return nil
}

// main
func main() {

	// creates a new file watcher
	watcher, _ = fsnotify.NewWatcher()
	defer watcher.Close()

	cpuCount := runtime.NumCPU()
	fileOperations = make(chan action, cpuCount)
	defer close(fileOperations)

	// starting at the root of the project, walk each file/directory searching for
	// directories
	directoryPath := os.Args[1]

	go handleFile()

	go func() {
		for {
			select {
			// watch for events
			case event := <-watcher.Events:
				determineAction(event.Name)
				// watch for errors
			case err := <-watcher.Errors:
				fmt.Println("ERROR", err)
			}
		}
	}()

	if err := filepath.Walk(directoryPath, watchDir); err != nil {
		fmt.Println("ERROR", err)
	}

	done := make(chan bool)
	<-done
}
