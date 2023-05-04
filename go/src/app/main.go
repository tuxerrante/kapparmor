package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"sort"
	"strings"
	"syscall"
	"time"
)

var (
	CONFIGMAP_PATH string = os.Getenv("PROFILES_DIR")
	POLL_TIME_ARG  string = os.Getenv("POLL_TIME")
	POLL_TIME      int
)

const (
	KERNEL_PATH         = "/sys/kernel/security/apparmor/profiles"
	PROFILER_BIN        = "/sbin/apparmor_parser"
	PROFILE_NAME_PREFIX = "custom."
	ETC_APPARMORD       = "/etc/apparmor.d/custom"
)

func main() {

	POLL_TIME = preFlightChecks()

	keepItRunning := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)

	log.Printf("> Polling directory %s every %d seconds.\n", CONFIGMAP_PATH, POLL_TIME)
	go pollProfiles(POLL_TIME, ctx, keepItRunning)

	// Manage OS signals for graceful shutdown
	go func() {
		<-signals
		log.Print("> Received stop signal, terminating..")

		// Stop polling new profiles
		cancel()

		// Delete all loaded profiles
		err := unloadAllProfiles()
		checkFatal(err)

		log.Print("> The Eagle has landed.")
	}()

	<-keepItRunning
}

// Every pollTime seconds it will read the mounted volume for profiles,
// it will call loadNewProfiles() then to check if they are new ones or not.
// Executed as go-routine it will run forever until a cancel() is called on the given context.
func pollProfiles(pollTime int, ctx context.Context, keepItRunning chan struct{}) {
	log.Print("> Polling started.")
	ticker := time.NewTicker(time.Duration(pollTime) * time.Second)
	pollNow := func() {
		newProfiles, err := loadNewProfiles()
		if err != nil {
			log.Fatalf(">> Error retrieving profiles: %v\n%v", newProfiles, err)
		}
	}

	for {
		select {
		case <-ctx.Done():
			keepItRunning <- struct{}{}
			return
		case <-ticker.C:
			// TODO: check if the function is still running before starting a new one
			// Ticker should not execute if already running
			pollNow()
		}
	}
}

// Check if the current profiles are really new and loads them after verifying some conditions
func loadNewProfiles() ([]string, error) {

	// Check profiles directory accessibility
	profilesAreReadable, newProfiles := getNewProfiles()
	if !profilesAreReadable {
		log.Fatalf(">> There was an error accessing the files in %s.\n", CONFIGMAP_PATH)
	}

	// TODO: improvable, customLoadedProfiles will always contain the new profiles recently created
	loadedProfiles, customLoadedProfiles, err := getLoadedProfiles()
	if err != nil {
		log.Fatalf(">> Error reading existing profiles.\n%v", err)
	}

	// Clean possible empty keys
	delete(customLoadedProfiles, "")
	delete(loadedProfiles, "")

	// Sort alphabetically the profiles and print them
	log.Printf("%d Profiles already on this node:", len(loadedProfiles))
	loadedProfileNames := make([]string, len(loadedProfiles))
	for loadedProfileName := range loadedProfiles {
		loadedProfileNames = append(loadedProfileNames, loadedProfileName)
	}
	sort.Strings(loadedProfileNames)
	for _, p := range loadedProfileNames {
		if p != "" {
			log.Printf("- %s\n", p)
		}
	}

	// Check if it exists a profile already loaded with the same name
	// TODO: it should contain filenames and not paths to be consistent with loadedProfilesToUnload
	newProfilesToApply := make([]string, 0, len(newProfiles))

	for newProfileName := range newProfiles {

		filePath1 := path.Join(CONFIGMAP_PATH, newProfileName)

		// It exists a loaded profile with the same name
		if customLoadedProfiles[newProfileName] {

			// If the profile is exactly the same skip the apply
			log.Printf("Checking %s profile..", path.Join(CONFIGMAP_PATH, newProfileName))
			contentIsTheSame, err := HasTheSameContent(nil, filePath1, path.Join(CONFIGMAP_PATH, newProfileName))
			if err != nil {
				// Error in checking the content of "/app/profiles/custom.deny-write-outside-app" VS "custom.deny-write-outside-app"
				log.Printf(">> Error in checking the content of %q VS %q\n", filePath1, newProfileName)
				return nil, err
			}
			if contentIsTheSame {
				log.Print("Contents seems the same, skipping..")
				continue
			}
		}
		newProfilesToApply = append(newProfilesToApply, filePath1)
	}

	// Unload custom profiles if they're in the filesystem but not in the configmap list
	loadedProfilesToUnload := make([]string, 0, len(customLoadedProfiles))
	for customLoadedProfile := range customLoadedProfiles {
		if !newProfiles[customLoadedProfile] {
			loadedProfilesToUnload = append(loadedProfilesToUnload, customLoadedProfile)
		}
	}

	// Execute apparmor_parser --replace --verbose filteredNewProfiles
	printLogSeparator()
	log.Println("> Apparmor REPLACE and apply new profiles..")
	for _, profilePath := range newProfilesToApply {
		err := loadProfile(profilePath)
		if err != nil {
			log.Printf("ERROR: %s", err)
		}
	}

	// Execute apparmor_parser --remove obsoleteProfilePath
	if len(loadedProfilesToUnload) > 0 {
		printLogSeparator()
		log.Println("> AppArmor REMOVE orphans profiles..")
		for _, profileFileName := range loadedProfilesToUnload {
			err := unloadProfile(profileFileName)
			if err != nil {
				log.Fatal(err)
			}
		}
	}

	log.Println("> Done!\n> Waiting next poll..")
	printLogSeparator()
	return newProfilesToApply, nil
}

// It reads the files provided in the CONFIGMAP_PATH
func getNewProfiles() (bool, map[string]bool) {
	return areProfilesReadable(CONFIGMAP_PATH)
}

// It reads a list of profile names from a singe file under KERNEL_PATH
func getLoadedProfiles() (map[string]bool, map[string]bool, error) {
	return getProfilesNamesFromFile(KERNEL_PATH, PROFILE_NAME_PREFIX)
}

// Search for profiles already present on the current node in '$apparmorfs/profiles' folder
// It returns two maps to split the custom profiles introduced by us and the built-ins in the node OS
// Output
//   - profiles{} map containing all the loaded profiles
//   - customProfiles{} map containing only the profiles starting with the given PREFIX
func getProfilesNamesFromFile(profilesPath, profileNamePrefix string) (map[string]bool, map[string]bool, error) {

	profilesFile, err := os.Open(profilesPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open %s: %v", profilesPath, err)
	}
	defer profilesFile.Close()

	profiles := map[string]bool{}
	customProfiles := map[string]bool{}

	scanner := bufio.NewScanner(profilesFile)

	for scanner.Scan() {
		profileName := parseProfileName(scanner.Text())
		if profileName == "" {
			continue
		}
		if strings.HasPrefix(profileName, PROFILE_NAME_PREFIX) {
			customProfiles[profileName] = true
		}
		profiles[profileName] = true
	}
	return profiles, customProfiles, nil
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

func loadProfile(profilePath string) error {
	err := execApparmor("--verbose", "--replace", profilePath)
	checkFatal(err)

	// Copy the profile definition in the apparmor configuration standard directory
	log.Printf("Copying profile in %s", ETC_APPARMORD)
	return CopyFile(profilePath, ETC_APPARMORD)
}

// Remove all custom profiles from the kernel, reading from ETC_APPARMORD folder
func unloadAllProfiles() error {
	dirEntries, err := os.ReadDir(ETC_APPARMORD)
	checkFatal(err)

	for _, entry := range dirEntries {
		if !entry.IsDir() && entry.Type().IsRegular() {
			err := unloadProfile(entry.Name())
			checkFatal(err)
		}
	}
	return nil
}

// Remove an apparmor profile from the kernel
func unloadProfile(fileName string) error {
	filePath := path.Join(ETC_APPARMORD, fileName)

	err := execApparmor("--verbose", "--remove", filePath)
	if err != nil {
		return err
	}
	return os.Remove(filePath)
}

func execApparmor(args ...string) error {
	cmd := exec.Command("apparmor_parser", args...)
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	out, err := cmd.Output()
	path := args[len(args)-1]
	log.Printf("Loading profiles from %s:\n%s", path, out)
	if err != nil {
		if stderr.Len() > 0 {
			log.Println(stderr.String())
		}
		return fmt.Errorf(" error loading profile! %v", err)
	}

	return nil
}

func checkFatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// Useless line separator to simplify logs reading.
func printLogSeparator() {
	log.Println("============================================================")
}
