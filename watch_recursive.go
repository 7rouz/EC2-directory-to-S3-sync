package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	concurrent "github.com/echaouchna/go-threadpool"
	"github.com/fsnotify/fsnotify"
)

var watcher *fsnotify.Watcher

type actionType string

const (
	copy   actionType = "copy"
	remove actionType = "remove"
)

func (actionType actionType) value() string {
	return string(actionType)
}

type action struct {
	path       string
	actionType actionType
}

// var fileOperations chan action
var (
	fileOperations chan concurrent.Action
	version        string
)

func copyFile(id int, value interface{}) {
	fmt.Printf("Adding! %#v\n", value)
}

func removeFile(id int, value interface{}) {
	fmt.Printf("Removing! %#v\n", value)
}

func determineAction(path string) {
	op := copy
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		op = remove
	}

	if op == remove || !fi.IsDir() {
		fileOperations <- concurrent.Action{Name: op.value(), Data: path}
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
	fileOperations = make(chan concurrent.Action, cpuCount)
	defer close(fileOperations)

	// starting at the root of the project, walk each file/directory searching for
	// directories
	directoryPath := os.Args[1]

	actionJobs := make(map[string]concurrent.JobFunc)
	actionJobs[copy.value()] = copyFile
	actionJobs[remove.value()] = removeFile
	_, _, stopWorkers := concurrent.RunWorkers(fileOperations, actionJobs, 0)

	defer stopWorkers()

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
