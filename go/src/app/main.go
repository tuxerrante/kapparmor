package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
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

	log.Printf("> Polling directory %s every %d seconds.\n", CONFIGMAP_PATH, POLL_TIME)

	pollProfiles(POLL_TIME)
}

// Profiles will probably change content while keeping the same name, so a digest comparison
// can be useful if we don't want to load everything every time.
func pollProfiles(delay int) {

	ticker := time.NewTicker(time.Duration(delay) * time.Second)
	pollNow := func() {
		newProfiles, err := loadNewProfiles()
		if err != nil {
			log.Fatalf(">> Error retrieving profiles: %v\n%v", newProfiles, err)
		}
	}

	pollNow()

	for range ticker.C {
		pollNow()
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

	// Clean eventually empty keys
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
	log.Println("============================================================")
	log.Println("> Apparmor replace and apply new profiles..")
	for _, profilePath := range newProfilesToApply {
		err := loadProfile(profilePath)
		if err != nil {
			log.Printf("ERROR: %s", err)
		}
	}

	// Execute apparmor_parser --remove obsoleteProfilePath
	if len(loadedProfilesToUnload) > 0 {
		log.Println("============================================================")
		log.Println("> AppArmor REMOVE orphans profiles..")
		for _, profileFileName := range loadedProfilesToUnload {
			err := unloadProfile(profileFileName)
			if err != nil {
				log.Fatal(err)
			}
		}
	}

	log.Println("> Done!\n> Waiting next poll..")
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
	if err != nil {
		log.Fatal(err)
	}

	// Copy the profile definition in the apparmor configuration standard directory
	log.Printf("Copying profile in %s", ETC_APPARMORD)
	return CopyFile(profilePath, ETC_APPARMORD)
}

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
