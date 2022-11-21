package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
)

var (
	DIRNAME       string = os.Getenv("PROFILES_DIR")
	POLL_TIME_ARG string = os.Getenv("POLL_TIME")
)

const (
	profiler = "/sbin/apparmor_parser"
)

func main() {

	// Type check
	POLL_TIME, err := strconv.Atoi(POLL_TIME_ARG)
	if err != nil {
		log.Fatalf("It was not possible to convert env var POLL_TIME %v to an integer.\n%v", POLL_TIME, err)
	}

	fmt.Printf("> Polling directory %s every %d seconds.\n", DIRNAME, POLL_TIME)

	// Check profiler binary
	// TODO: check also for permissions
	if _, err := os.Stat(profiler); os.IsNotExist(err) {
		log.Fatal(err)
	}

	// Check profiles are readable
	if !areProfilesReadable() {
		log.Fatalf("There was an error accessing the files in %s.\n", DIRNAME)
	}

	// Poll configmap forever every POLL_TIME seconds
	pollProfiles(POLL_TIME)
}

func areProfilesReadable() bool {

	files, err := os.ReadDir(DIRNAME)
	if err != nil {
		log.Fatal(err.Error())
	}

	// TODO: Should the app terminate if no profiles are present?
	if len(files) == 0 {
		fmt.Printf("No files were found in the given folder!\n")
		return true
	}

	fmt.Printf("Found files in given folder:\n")
	for _, file := range files {
		if file.IsDir() {
			fmt.Printf("Directory '%s' will be skipped.\n", file.Name())
			continue
		}
		fmt.Printf("- %s\n", file.Name())
	}

	return true
}

func pollProfiles(delay int) {
	fmt.Printf("> pollProfiles TBD %d\n", delay)
}
