package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
)

func main() {
	// TODO: add default values if the env var is missing
	var DIRNAME string  = os.Getenv("PROFILES_DIR")
	// TODO: check value before converting to int
	POLL_TIME, err := strconv.Atoi(os.Getenv("POLL_TIME"))
	if err != nil {
		log.Fatalf("Was not possible to convert env var POLL_TIME %v to an integer", POLL_TIME)
	}

	log.Printf("Polling directory %s every %d seconds.", DIRNAME, POLL_TIME)

	files, err := os.ReadDir(DIRNAME)
	if err != nil  {
		log.Fatal(err.Error())
	}

	for _, file := range files {
		if file.IsDir() {
			fmt.Printf("Directory '%s' will be skipped.\n", file.Name())
			continue
		}
		fmt.Printf("- %s\n", file.Name())		
	}

	// TODO: filter only the files recently changed
	// 	Is not safe to count only the current time - POLL_TIME since the last scan could have been failed.

	// TODO: move or directly apply profiles
}