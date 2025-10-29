// Package main provides AppArmor profile and filesystem operations for kapparmor.
package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// isSafePath checks for path traversal and absolute path issues
func isSafePath(p string) bool {
	clean := filepath.Clean(p)
	// Allow relative paths (not starting with /)
	if !filepath.IsAbs(clean) {
		return !strings.Contains(clean, "..")
	}
	// Allow only specific absolute path prefixes
	allowedPrefixes := []string{"/app/", "/etc/", "/sys/kernel/security/apparmor/"}
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(clean, prefix) {
			return true
		}
	}
	return false
}

func preFlightChecks() (int, error) {
	// Environment variable type check
	pollTime, err := strconv.Atoi(PollTimeArg)
	if err != nil {
		return 0, fmt.Errorf(">> It was not possible to convert env var POLL_TIME %v to an integer.\n%v", pollTime, err)
	}
	if pollTime < 1 {
		log.Printf("warning, POLL_TIME %v too low! Defaulting to 1 second.", pollTime)
		pollTime = 1
	}
	if pollTime > MaxAllowedPollingTime {
		return 0, fmt.Errorf(">> Too high value for POLL_TIME (%v). Please set a number between 0 and %d", pollTime, MaxAllowedPollingTime)
	}

	// Check profiler binary
	if _, err := os.Stat(ProfilerFullPath); os.IsNotExist(err) {
		return 0, err
	}

	// Check if custom directory exists, creates it otherwise
	if _, err := os.Stat(EtcApparmord); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(EtcApparmord, 0o750)
		if err != nil {
			return 0, err
		}
		log.Printf("> Directory %s created.", EtcApparmord)
	}

	return pollTime, nil
}

// Compare the byte content of two given files
// The function supports also an external filesystem for testing and future usages
func HasTheSameContent(fsys fs.FS, filePath1, filePath2 string) (bool, error) {
	var file1, file2 os.FileInfo

	// Checking files on current filesystem
	if fsys == nil {
		// Validate file paths before reading
		if !isSafePath(filePath1) || !isSafePath(filePath2) {
			return false, fmt.Errorf("unsafe file path detected")
		}
		fileBytes1, err := os.ReadFile(filePath1) // #nosec G304 -- path validated by isSafePath
		if err != nil {
			log.Fatal(err)
		}
		fileBytes2, err := os.ReadFile(filePath2) // #nosec G304 -- path validated by isSafePath
		if err != nil {
			log.Fatal(err)
		}
		if !bytes.Equal(fileBytes1, fileBytes2) {
			return false, nil
		}
		return true, nil
	}

	// dir will contain the files in given filesystem
	dir, err := fs.ReadDir(fsys, ".")
	if err != nil {
		log.Printf("ERROR in opening directory %v\n", fsys)
		return false, err
	}

	log.Printf(" First file path: %v, Second file path: %v", filePath1, filePath2)

	for _, file := range dir {
		if filePath1 == file.Name() {
			file1, _ = file.Info()
		} else if filePath2 == file.Name() {
			file2, _ = file.Info()
		}
	}

	if file1 == nil || file2 == nil {
		return false, fmt.Errorf("ERROR: files not found")
	}

	if file1.Size() != file2.Size() {
		return false, nil
	}

	f1, err := fsys.Open(file1.Name())
	if err != nil {
		return false, err
	}
	defer func() {
		err := f1.Close()
		if err != nil {
			log.Printf("error closing file1: %v", err)
		}
	}()

	f2, err := fsys.Open(file2.Name())

	if err != nil {
		return false, err
	}
	defer func() {
		err := f2.Close()
		if err != nil {
			log.Printf("error closing file2: %v", err)
		}
	}()

	return compareBytes(f1, f2)
}

func compareBytes(f1, f2 fs.File) (bool, error) {
	data1, err := io.ReadAll(f1)
	if err != nil {
		return false, err
	}

	data2, err := io.ReadAll(f2)
	if err != nil {
		return false, err
	}

	if !bytes.Equal(data1, data2) {
		return false, nil
	}

	return true, nil
}
func areProfilesReadable(folderName string) (bool, map[string]bool) {
	filenames := map[string]bool{}
	files, err := os.ReadDir(folderName)
	if err != nil {
		log.Fatal(err.Error())
	}

	if len(files) == 0 {
		log.Printf("No files were found in the given folder!\n")
		return false, nil
	}

	log.Printf("Found files in %s:\n", folderName)
	for _, file := range files {
		filename := file.Name()
		if file.IsDir() {
			log.Printf("Directory '%s' will be skipped.\n", filename)
			continue
		} else if strings.HasPrefix(filename, ".") {
			log.Printf("'%s' will be skipped.\n", filename)
			continue
		}

		if err := IsProfileNameCorrect(folderName, filename); err != nil {
			log.Fatalf("Profile name and filename '%s'are not the same: %s", filename, err)
		}

		log.Printf("- %s\n", filename)
		filenames[filename] = true
	}

	return true, filenames
}

// isProfileNameCorrect returns true if the filename is the same as the profile name
func IsProfileNameCorrect(directory, filename string) error {
	var isProfileWordPresent = false
	var fileProfileName string

	// input validation
	if ok, err := isValidPath(directory); !ok {
		return err
	}
	if ok, err := isValidFilename(filename); !ok {
		return err
	}

	// Check if the file doesn't exist
	profilePath := path.Join(directory, filename)
	if !isSafePath(profilePath) {
		return fmt.Errorf("unsafe file path detected: %s", profilePath)
	}
	if _, err := os.Stat(profilePath); errors.Is(err, os.ErrNotExist) {
		return err
	}

	// Open the file to get a scanner to search for text later
	fileReader, err := os.Open(profilePath) // #nosec G304 -- path validated by isSafePath
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(fileReader)

	// Validate the syntax
	// the first index of a curly bracket should be greater than the first occurrence of "profile"
	fileBytes, err := os.ReadFile(profilePath) // #nosec G304 -- path validated by isSafePath
	checkPanic(err)

	profileIndex := bytes.Index(fileBytes, []byte("profile"))
	curlyBracketIndex := bytes.Index(fileBytes, []byte("{"))
	if curlyBracketIndex < 0 || curlyBracketIndex < profileIndex {
		return errors.New("couldn't find a { after 'profile' keyword")
	}

	// Search for line starting with 'profile' word
	for scanner.Scan() {
		fileLine := scanner.Text()
		fileProfileNameSlice := strings.Split(fileLine, " ")

		// log.Printf("Checking line: %s", fileLine)
		// log.Println(fileProfileNameSlice)

		// searching for a line with a least three tokens
		if len(fileProfileNameSlice) < 2 || fileProfileNameSlice[0] != "profile" {
			continue
		}
		// If the line starts with 'profile' check the following name
		fileProfileName = strings.TrimSpace(fileProfileNameSlice[1])
		isProfileWordPresent = true
		defer func() {
			err := fileReader.Close()
			if err != nil {
				log.Printf("error closing fileReader: %v", err)
			}
		}()
		log.Printf("Found profile name: %s", fileProfileName)
		break
	}

	if !isProfileWordPresent {
		return fmt.Errorf("there is an issue with the '%s' profile name, please check if the syntax is 'profile custom.YourName { ... }' or check again Unattached Profiles definition at https://documentation.suse.com/sles/15-SP1/html/SLES-all/cha-apparmor-profiles.html#sec-apparmor-profiles-types-unattached", filename)
	}

	if filename != fileProfileName {
		return fmt.Errorf("filename '%s' and profile name '%s' seems to be different", filename, fileProfileName)
	}

	return nil
}

func isValidPath(path string) (bool, error) {
	if len(path) == 0 {
		return false, fmt.Errorf("empty directory name")
	}

	cleanPath := filepath.Clean(path)
	substrings := strings.Split(cleanPath, string(os.PathSeparator))

	for _, substring := range substrings {
		// '.' is a valid path name but not a valid filename
		if len(substring) == 1 && substring[0] == '.' {
			return true, nil
		}
		if ok, err := isValidFilename(substring); !ok {
			return false, err
		}
	}
	return true, nil
}

/*
Accepts filenames that are:
  - not empty
  - not more than 255 chars long
  - not made of symbols excluding those one in 'validSymbols'
    e.g.: '@', '#', '§', '!', ' '
  - not made up of consecutive symbols
    e.g.: '..', '-_-'
    '..' paths are managed by filepath.Clean()
*/
func isValidFilename(filename string) (bool, error) {
	if len(filename) == 0 {
		return false, fmt.Errorf("empty filename")
	}

	if len(filename) == 1 && filename[0] == '.' {
		return false, fmt.Errorf("%q is not a valid filename", filename)
	}

	if len(filename) >= 255 {
		return false, fmt.Errorf("file name too long")
	}

	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return false, fmt.Errorf("invalid filename format")
	}

	isAlphaNumeric := func(char rune) bool {
		return unicode.IsDigit(char) || unicode.IsLetter(char)
	}

	// Restrict the filename to contain only commonly used chars
	validSymbols := []rune{'_', '-', '.'}
	var previousCharIsASymbol bool

	for i, char := range filename {
		if i > 0 && isCharInSlice(char, validSymbols) && previousCharIsASymbol {
			return false, fmt.Errorf("rejected suspect filename")
		}
		if !isAlphaNumeric(char) && !isCharInSlice(char, validSymbols) {
			return false, fmt.Errorf("invalid characters in filename")
		} else if isCharInSlice(char, validSymbols) {
			previousCharIsASymbol = true
		} else {
			previousCharIsASymbol = false
		}
	}
	return true, nil
}

func isCharInSlice(char rune, slice []rune) bool {
	for _, c := range slice {
		if char == c {
			return true
		}
	}
	return false
}

// CopyFile copies a file from src to dst. If src and dst files exist, and are
// the same, then return success. Otherwise, attempt to create a hard link
// between the two files. If that fail, copy the file contents from src to dst.
// Credits: https://stackoverflow.com/a/21067803/3673430
func CopyFile(src, dst string) error {
	// dst is the destination directory
	srcFileName := filepath.Base(src)
	dstCompleteFileName := path.Join(dst, srcFileName)

	sfi, err := os.Stat(src)
	if err != nil {
		log.Fatal(err)
	}

	if !sfi.Mode().IsRegular() {
		// cannot copy non-regular files (e.g., directories symlinks, devices, etc.)
		return fmt.Errorf("CopyFile: non-regular source file %s (%q)", sfi.Name(), sfi.Mode().String())
	}

	dfi, err := os.Stat(dstCompleteFileName)
	if err != nil {
		log.Print(err)
	} else {
		if !(dfi.Mode().IsRegular()) {
			return fmt.Errorf("CopyFile: non-regular destination file %s (%q)", dfi.Name(), dfi.Mode().String())
		}
		if os.SameFile(sfi, dfi) {
			log.Printf("File %s is already present", dstCompleteFileName)
			return nil
		}
	}

	if err = os.Link(src, dstCompleteFileName); err == nil {
		log.Printf("Hard link created in %s", dstCompleteFileName)
		return nil
	}

	log.Printf("Copying %s in %s", src, dstCompleteFileName)
	return copyFileContents(src, dstCompleteFileName)
}

// copyFileContents copies the contents of the file named src to the file named
// by dst. The file will be created if it does not already exist. If the
// destination file exists, all it's contents will be replaced by the contents
// of the source file.
func copyFileContents(src, dst string) (err error) {
	if !isSafePath(src) || !isSafePath(dst) {
		log.Print("unsafe file path detected in copyFileContents")
		return fmt.Errorf("unsafe file path detected")
	}
	in, err := os.Open(src) // #nosec G304 -- path validated by isSafePath
	if err != nil {
		log.Print(err)
		return
	}
	defer func() {
		if err := in.Close(); err != nil {
			log.Printf("error closing input file: %v", err)
		}
	}()

	out, err := os.Create(dst) // #nosec G304 -- path validated by isSafePath
	if err != nil {
		log.Print(err)
		return
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		log.Print(err)
		return
	}

	err = out.Sync()
	return
}
