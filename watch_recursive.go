package main

import (
	"crypto/sha256"
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	concurrent "github.com/echaouchna/go-threadpool"
	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
)

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
	version  bool
	src      string
	dest     string
	logLevel string
}

type safeDirList struct {
	v   []string
	mux sync.Mutex
}

type safeFileHashMap struct {
	v   map[string]string
	mux sync.Mutex
}

var (
	dirList        = safeDirList{[]string{}, sync.Mutex{}}
	fileHashMap    = safeFileHashMap{map[string]string{}, sync.Mutex{}}
	fileOperations chan concurrent.Action
	logLevels      = map[string]logrus.Level{
		"panic": log.PanicLevel,
		"fatal": log.FatalLevel,
		"error": log.ErrorLevel,
		"warn":  log.WarnLevel,
		"info":  log.InfoLevel,
		"debug": log.DebugLevel,
		"trace": log.TraceLevel,
	}
	opts    cliOptions
	version string
	watcher *fsnotify.Watcher
)

func init() {
	flag.BoolVar(&opts.version, "version", false, "Prints version")
	flag.StringVar(&opts.dest, "s3", "", "S3 destination")
	flag.StringVar(&opts.src, "src", "", "Local source directory")
	flag.StringVar(&opts.logLevel, "log-level", "info", "Local source directory")
}

func copyFile(id int, value interface{}) {
	log.Infof("Copying file! %#v\n", value)
}

func removeFile(id int, value interface{}) {
	log.Infof("Removing file! %#v\n", value)
}

func removePathFromWatchList(path string) (removed bool) {
	removed = false
	dirList.mux.Lock()
	defer dirList.mux.Unlock()
	found, position := stringInSlice(dirList.v, path)
	if found {
		log.Debugf("Removing from watch list <%s>", path)
		dirList.v = removeFromSlice(position, dirList.v)
		watcher.Remove(path)
		removed = true
	}
	return
}

func determineAction(path string) {
	op := copy

	if path == "" {
		return
	}

	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		op = remove
	}

	if op == remove {
		if folderRemoved := removePathFromWatchList(path); folderRemoved {
			return
		}
	}

	if op == copy && fi.IsDir() {
		watchDirectory(path)
		return
	}

	if op == copy && !fi.IsDir() {
		hasher := sha256.New()
		s, err := ioutil.ReadFile(path)
		if err != nil {
			log.Fatal(err)
		}
		hasher.Write(s)
		sha := base64.URLEncoding.EncodeToString(hasher.Sum(nil))
		if val, ok := fileHashMap.v[path]; ok {
			if val == sha {
				return
			}
		}
		fileHashMap.mux.Lock()
		fileHashMap.v[path] = sha
		fileHashMap.mux.Unlock()
	}

	if op == remove {
		if _, ok := fileHashMap.v[path]; ok {
			fileHashMap.mux.Lock()
			delete(fileHashMap.v, path)
			fileHashMap.mux.Unlock()
		}
	}
	actionToSend := concurrent.Action{Name: op.value(), Data: path}
	log.Debugf("Sending operation %v", actionToSend)
	fileOperations <- actionToSend
}

func stringInSlice(a []string, x string) (found bool, position int) {
	position = -1
	found = false
	i, j := 0, len(a)
	for i < j {
		h := int(uint(i+j) >> 1)
		if a[h] == x {
			position = h
			found = true
			return
		}
		if a[h] < x {
			i = h + 1
		} else {
			j = h
		}
	}
	return
}

func removeFromSlice(index int, a []string) []string {
	return append(a[:index], a[index+1:]...)
}

// doWalk gets run as a walk func, searching for directories to add watchers to
func doWalk(path string, fi os.FileInfo, err error) error {
	// since fsnotify can watch all the files in a directory, watchers only need
	// to be added to each nested directory
	if fi.IsDir() {
		dirList.mux.Lock()
		defer dirList.mux.Unlock()
		found, _ := stringInSlice(dirList.v, path)
		if !found {
			log.Debugf("Adding to watch list <%s>", path)
			dirList.v = append(dirList.v, path)
			sort.Strings(dirList.v)
			watcher.Add(path)
		} else {
			log.Debugf("Skipping <%s>: already watching it", path)
		}
		return watcher.Add(path)
	}

	determineAction(path)
	return nil
}

func watchDirectory(path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		removePathFromWatchList(path)
	} else if err := filepath.Walk(path, doWalk); err != nil {
		log.Errorf("Error walking folder <%s>: %v", path, err)
	}
}

func readWatcherNotifications() {
	func() {
		for {
			select {
			// watch for events
			case event := <-watcher.Events:
				determineAction(event.Name)
				// watch for errors
			case err := <-watcher.Errors:
				log.Errorln("Error reading notifications", err)
			}
		}
	}()
}

func validateOptions() {
	if _, err := os.Stat(opts.src); os.IsNotExist(err) {
		log.Errorf("src [%s] error: %v", opts.src, err)
		os.Exit(1)
	}

	var logLevel log.Level

	if val, ok := logLevels[opts.logLevel]; ok {
		logLevel = val
	} else {
		log.Warnf("Log level <%s> does not exist, should be one of [panic, fatal, error, warn, info, debug, trace]", opts.logLevel)
		log.Warnln("Log level is set to info")
		logLevel = log.InfoLevel
	}

	log.SetLevel(logLevel)
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
	srcDirectoryPath := opts.src

	JobFunc := make(map[string]concurrent.JobFunc)
	JobFunc[copy.value()] = copyFile
	JobFunc[remove.value()] = removeFile
	_, _, stopWorkers := concurrent.RunWorkers(fileOperations, JobFunc, 0)

	defer stopWorkers()

	go readWatcherNotifications()

	watchDirectory(srcDirectoryPath)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	timerMux := sync.Mutex{}
	running := false

	for {
		select {
		case <-ticker.C:
			if !running {
				running = true
				go func() {
					timerMux.Lock()
					defer timerMux.Unlock()
					watchDirectory(srcDirectoryPath)
					running = false
				}()
			}
		}
	}
}
