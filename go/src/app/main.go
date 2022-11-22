package main

//
// TODO:
//  - unload profiles present only in loadedProfiles and not in NewProfiles
//  - manage symlinks: on the node there could be already some custom profile
//  - how to manage all default profiles present on the nodes after installing apparmor?
//      they're too many to have them all in one configmap
import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"golang.org/x/exp/maps"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	CONFIGMAP_PATH string = os.Getenv("PROFILES_DIR")
	POLL_TIME_ARG  string = os.Getenv("POLL_TIME")
)

const (
	KERNEL_PATH  = "/sys/kernel/security/apparmor/profiles"
	PROFILER_BIN = "/sbin/apparmor_parser"
)

func main() {

	// Type check
	POLL_TIME, err := strconv.Atoi(POLL_TIME_ARG)
	if err != nil {
		log.Fatalf(">> It was not possible to convert env var POLL_TIME %v to an integer.\n%v", POLL_TIME, err)
	}

	fmt.Printf("> Polling directory %s every %d seconds.\n", CONFIGMAP_PATH, POLL_TIME)

	// Check profiler binary
	if _, err := os.Stat(PROFILER_BIN); os.IsNotExist(err) {
		log.Fatal(err)
	}

	// Check profile directory accessibility
	profilesAreReadable, _ := areProfilesReadable(CONFIGMAP_PATH)
	if !profilesAreReadable {
		log.Fatalf(">> There was an error accessing the files in %s.\n", CONFIGMAP_PATH)
	}

	// Poll configmap forever every POLL_TIME seconds
	pollProfiles(POLL_TIME)
}

func areProfilesReadable(FOLDER_NAME string) (bool, []string) {

	filenames := []string{}
	files, err := os.ReadDir(CONFIGMAP_PATH)
	if err != nil {
		log.Fatal(err.Error())
	}

	// TODO: Should the app terminate if no profiles are present?
	if len(files) == 0 {
		fmt.Printf("No files were found in the given folder!\n")
		return true, nil
	}

	fmt.Printf("Found files in given folder:\n")
	for _, file := range files {
		if file.IsDir() {
			fmt.Printf("Directory '%s' will be skipped.\n", file.Name())
			continue
		}
		fmt.Printf("- %s\n", file.Name())
		filenames = append(filenames, file.Name())
	}

	return true, filenames
}

// Profiles will probably change content while keeping the same name, so a digest comparison
// can be very useful if we don't want to load everything every time.
// https://pkg.go.dev/github.com/opencontainers/go-digest#section-readme
func pollProfiles(delay int) {

	ticker := time.NewTicker(time.Duration(delay) * time.Second)
	pollNow := func() {
		newProfiles, err := loadNewProfiles()
		if err != nil {
			log.Fatalf(">> Error retrieving profiles: %v\n%v", newProfiles, err)
		}
	}
	for range ticker.C {
		pollNow()
	}
}

// Check if the current profiles are really new
func loadNewProfiles() ([]string, error) {
	loadedProfiles, err := getLoadedProfiles()
	if err != nil {
		log.Fatalf(">> Error reading existing profiles.\n%v", err)
	}

	fmt.Println("Profiles already on this node:")
	for profileName := range loadedProfiles {
		fmt.Printf("- %s", profileName)
	}

	newProfiles, err := getNewProfiles()
	if err != nil {
		log.Fatalf(">> Error reading new profiles in the configmap!\n%v", err)
	}
	fmt.Println("> newProfiles", strings.Join(maps.Keys(newProfiles), "\n"))

	filteredNewProfiles := make([]string, len(newProfiles))
	for k := range newProfiles {
		if loadedProfiles[k] == nil || !bytes.Equal(newProfiles[k], loadedProfiles[k]) {
			filteredNewProfiles = append(filteredNewProfiles, k)
		}
	}
	fmt.Println("> filteredNewProfiles", strings.Join(filteredNewProfiles, "\n"))

	obsoleteProfiles := make([]string, len(loadedProfiles))
	for k := range loadedProfiles {
		if newProfiles[k] == nil {
			obsoleteProfiles = append(obsoleteProfiles, k)
		}
	}
	fmt.Println("> obsoleteProfiles", strings.Join(obsoleteProfiles, "\n"))

	// Execute apparmor_parser --replace --verbose filteredNewProfiles
	fmt.Println("> TODO: apparmor_parser --replace ", filteredNewProfiles)

	// Execute apparmor_parser -R obsoleteProfiles
	fmt.Println("> TODO: apparmor_parser -R ", obsoleteProfiles)

	return filteredNewProfiles, nil
}

func getNewProfiles() (map[string][]byte, error) {
	return getProfiles(CONFIGMAP_PATH)
}
func getLoadedProfiles() (map[string][]byte, error) {
	return getProfiles(KERNEL_PATH)
}

// Search for profiles already present on the current node in '$apparmorfs/profiles' folder
// - to be more efficient I could create a profiles map with a capacity as big as the number of files in the folder
func getProfiles(profilesPath string) (map[string][]byte, error) {

	profilesFile, err := os.Open(profilesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %v", profilesPath, err)
	}
	defer profilesFile.Close()

	profiles := map[string][]byte{}
	currentHash := sha256.New()
	scanner := bufio.NewScanner(profilesFile)

	for scanner.Scan() {
		profileName := parseProfileName(scanner.Text())
		if profileName == "" {
			// Unknown line format; skip it.
			continue
		}
		// TODO: save the digest of the profile in the new map
		if _, err := io.Copy(currentHash, profilesFile); err != nil {
			log.Fatal(err)
		}
		// profiles[profileName] = true
		profiles[profileName] = currentHash.Sum(nil)
	}
	return profiles, nil
}

// The profiles file is formatted with one profile per line, matching a form:
//
//	namespace://profile-name (mode)
//	profile-name (mode)
//
// Where mode is {enforce, complain, kill}. The "namespace://" is only included for namespaced
// profiles. For the purposes of Kubernetes, we consider the namespace part of the profile name.
func parseProfileName(profileLine string) string {
	modeIndex := strings.IndexRune(profileLine, '(')
	if modeIndex < 0 {
		return ""
	}
	return strings.TrimSpace(profileLine[:modeIndex])
}
