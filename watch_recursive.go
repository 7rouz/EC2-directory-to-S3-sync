package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	concurrent "github.com/echaouchna/go-threadpool"
	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
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

type cliOptions struct {
	version bool
	src     string
	dest    string
}

// var fileOperations chan action
var (
	fileOperations chan concurrent.Action
	version        string
	opts           cliOptions
)

func init() {
	flag.BoolVar(&opts.version, "version", false, "Prints version")
	flag.StringVar(&opts.dest, "s3", "", "S3 destination")
	flag.StringVar(&opts.src, "src", "", "Local source directory")
}

func copyFile(id int, value interface{}) {
	log.Infof("Adding! %#v\n", value)
}

func removeFile(id int, value interface{}) {
	log.Infof("Removing! %#v\n", value)
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
		log.Errorln("ERROR", err)
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

func validateOptions() {
	if _, err := os.Stat(opts.src); os.IsNotExist(err) {
		log.Errorln("src error:", err)
		os.Exit(1)
	}
}

// main
func main() {
	flag.Parse()

	if opts.version {
		fmt.Println(version)
		os.Exit(0)
	}

	validateOptions()

	// creates a new file watcher
	watcher, _ = fsnotify.NewWatcher()
	defer watcher.Close()

	cpuCount := runtime.NumCPU()
	fileOperations = make(chan concurrent.Action, cpuCount)
	defer close(fileOperations)

	// starting at the root of the project, walk each file/directory searching for
	// directories
	directoryPath := opts.src

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
				log.Errorln("ERROR", err)
			}
		}
	}()

	if err := filepath.Walk(directoryPath, watchDir); err != nil {
		log.Errorln("ERROR", err)
	}

	done := make(chan bool)
	<-done
}
