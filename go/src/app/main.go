package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	CONFIGMAP_PATH string = os.Getenv("PROFILES_DIR")
	POLL_TIME_ARG  string = os.Getenv("POLL_TIME")
)

const (
	KERNEL_PATH         = "/sys/kernel/security/apparmor/profiles"
	PROFILER_BIN        = "/sbin/apparmor_parser"
	PROFILE_NAME_PREFIX = "custom."
	ETC_APPARMORD       = "/etc/apparmor.d/custom"
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

	// Poll configmap forever every POLL_TIME seconds
	pollProfiles(POLL_TIME)
}

func areProfilesReadable(FOLDER_NAME string) (bool, map[string]bool) {

	filenames := map[string]bool{}
	files, err := os.ReadDir(FOLDER_NAME)
	if err != nil {
		log.Fatal(err.Error())
	}

	if len(files) == 0 {
		fmt.Printf("No files were found in the given folder!\n")
		return false, nil
	}

	fmt.Printf("Found files in given folder:\n")
	for _, file := range files {
		if file.IsDir() {
			fmt.Printf("Directory '%s' will be skipped.\n", file.Name())
			continue
		}
		fmt.Printf("- %s\n", file.Name())
		filenames[file.Name()] = true
	}

	return true, filenames
}

// Profiles will probably change content while keeping the same name, so a digest comparison
// can be very useful if we don't want to load everything every time.
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

	loadedProfiles, customLoadedProfiles, err := getLoadedProfiles()
	if err != nil {
		log.Fatalf(">> Error reading existing profiles.\n%v", err)
	}

	fmt.Println("Profiles already on this node:")
	for loadedProfileName := range loadedProfiles {
		fmt.Printf("- %s\n", loadedProfileName)
	}

	// Check if it exists a profile already loaded with the same name
	// TODO: it should contains filenames and not paths to be consistent with loadedProfilesToUnload
	newProfilesToApply := make([]string, 0, len(newProfiles))

	for newProfileName := range newProfiles {

		filePath1 := path.Join(CONFIGMAP_PATH, newProfileName)

		// It exists a loaded profile with the same name
		if customLoadedProfiles[newProfileName] {

			// If the profile is exactly the same skip the apply
			filePath2 := path.Join(KERNEL_PATH, newProfileName)
			contentIsTheSame, err := hasTheSameContent(filePath1, filePath2)
			if err != nil {
				fmt.Printf(">> Error in checking the content of %s VS %s", filePath1, filePath2)
				return nil, err
			}
			if contentIsTheSame {
				fmt.Printf("Content of  %s and %s seems the same, skipping.", filePath1, filePath2)
				continue
			}
		}
		newProfilesToApply = append(newProfilesToApply, filePath1)
	}

	// TODO: check for profiles to unload
	loadedProfilesToUnload := make([]string, 0, len(customLoadedProfiles))
	for customLoadedProfile := range customLoadedProfiles {
		if !newProfiles[customLoadedProfile] {
			loadedProfilesToUnload = append(loadedProfilesToUnload, customLoadedProfile)
		}
	}

	// Execute apparmor_parser --replace --verbose filteredNewProfiles
	fmt.Println("============================================================")
	fmt.Println("> Apparmor replace and apply new profiles..")
	for _, profilePath := range newProfilesToApply {
		loadProfile(profilePath)
	}

	// Execute apparmor_parser --remove obsoleteProfilePath
	fmt.Println("============================================================")
	fmt.Println("> AppArmor REMOVE orphans profiles..")
	for _, profileFileName := range loadedProfilesToUnload {
		unloadProfile(profileFileName)
	}

	fmt.Println("> Done!\n> Waiting next poll..")
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

func hasTheSameContent(filePath1, filePath2 string) (bool, error) {
	// compare sizes
	file1, openErr1 := os.Open(filePath1)
	if openErr1 != nil {
		return false, openErr1
	}
	defer file1.Close()

	file1_info, err := file1.Stat()
	if err != nil {
		log.Fatal("> Error accessing stats from file ", filePath1)
	}

	file2, openErr2 := os.Open(filePath2)
	if openErr2 != nil {
		return false, openErr2
	}
	defer file2.Close()

	file2_info, err := file2.Stat()
	if err != nil {
		log.Fatal("> Error accessing stats from file ", filePath2)
	}

	if file1_info.Size() != file2_info.Size() {
		return false, nil
	}

	// compare content through a sha256 hash
	h1 := sha256.New()
	if _, err := io.Copy(h1, file1); err != nil {
		log.Fatal("Error in generating a hash for ", filePath1)
	}

	h2 := sha256.New()
	if _, err := io.Copy(h2, file2); err != nil {
		log.Fatal("Error in generating a hash for ", filePath2)
	}

	// Sum appends the current hash to nil and returns the resulting slice
	if bytes.Equal(h1.Sum(nil), h2.Sum(nil)) {
		fmt.Printf("> Hashes are different\n %s: %s\n %s: %s", filePath1, h1, filePath2, h2)
		return false, nil
	}

	return true, nil
}

func loadProfile(profilePath string) error {
	execApparmor("--verbose", "--replace", profilePath)
	// Copy the profile definition in the apparmor configuration standard directory
	return CopyFile(profilePath, ETC_APPARMORD)
}

func unloadProfile(fileName string) error {
	filePath := path.Join(ETC_APPARMORD, fileName)
	execApparmor("--verbose", "--remove", filePath)
	return os.Remove(filePath)
}

func execApparmor(args ...string) error {
	cmd := exec.Command("apparmor_parser", args...)
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	out, err := cmd.Output()
	path := args[len(args)-1]
	fmt.Printf("Loading profiles from %s:\n%s", path, out)
	if err != nil {
		if stderr.Len() > 0 {
			fmt.Println(stderr.String())
		}
		return fmt.Errorf("error loading profile! %v", err)
	}

	return nil
}

// CopyFile copies a file from src to dst. If src and dst files exist, and are
// the same, then return success. Otherwise, attempt to create a hard link
// between the two files. If that fail, copy the file contents from src to dst.
// Credits: https://stackoverflow.com/a/21067803/3673430
func CopyFile(src, dst string) (err error) {
	sfi, err := os.Stat(src)
	if err != nil {
		return
	}
	if !sfi.Mode().IsRegular() {
		// cannot copy non-regular files (e.g., directories symlinks, devices, etc.)
		return fmt.Errorf("CopyFile: non-regular source file %s (%q)", sfi.Name(), sfi.Mode().String())
	}
	dfi, err := os.Stat(dst)
	if err != nil {
		if !os.IsNotExist(err) {
			return
		}
	} else {
		if !(dfi.Mode().IsRegular()) {
			return fmt.Errorf("CopyFile: non-regular destination file %s (%q)", dfi.Name(), dfi.Mode().String())
		}
		if os.SameFile(sfi, dfi) {
			return
		}
	}
	if err = os.Link(src, dst); err == nil {
		return
	}
	err = copyFileContents(src, dst)
	return
}

// copyFileContents copies the contents of the file named src to the file named
// by dst. The file will be created if it does not already exist. If the
// destination file exists, all it's contents will be replaced by the contents
// of the source file.
func copyFileContents(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return
	}
	err = out.Sync()
	return
}
