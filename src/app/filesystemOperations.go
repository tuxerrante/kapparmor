// Package main provides AppArmor profile and filesystem operations for kapparmor.
package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// isSafePath checks for path traversal and absolute path issues.
func isSafePath(p string) bool {
	cleanFS := filepath.Clean(p)
	// Normalize to forward slashes for prefix checks
	clean := filepath.ToSlash(cleanFS)
	// Consider Unix-style absolute paths even on Windows
	isAbs := filepath.IsAbs(cleanFS) || strings.HasPrefix(clean, "/")
	if !isAbs {
		return !strings.Contains(clean, "..")
	}
	// Allow only specific absolute path prefixes
	allowedPrefixes := []string{"/app/", "/etc/", "/sys/kernel/security/apparmor/", "/tmp/"}
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(clean, prefix) {
			return true
		}
	}

	return false
}

func preFlightChecks(cfg *AppConfig) (int, error) {
	// Environment variable type check
	pollTime, err := strconv.Atoi(cfg.PollTimeArg)
	if err != nil {
		return 0, fmt.Errorf(
			">> It was not possible to convert env var POLL_TIME %v to an integer. Error: %v",
			pollTime,
			err)
	}

	if pollTime < 1 {
		slog.Default().Warn("POLL_TIME too low, defaulting to 1 second", slog.Int("value", pollTime))
		pollTime = 1
	}

	if pollTime > MaxAllowedPollingTime {
		return 0, fmt.Errorf(
			">> Too high value for POLL_TIME (%v). Please set a number between 0 and %d",
			pollTime,
			MaxAllowedPollingTime)
	}

	// Check profiler binary (support /usr/sbin and /sbin)
	if _, err := os.Stat(cfg.ProfilerFullPath); os.IsNotExist(err) {
		candidates := []string{"/usr/sbin/" + ProfilerBin, "/sbin/" + ProfilerBin}

		var found string

		for _, c := range candidates {
			if _, e := os.Stat(c); e == nil {
				found = c

				break
			}
		}

		if found == "" {
			return 0, err
		}

		cfg.ProfilerFullPath = found
		slog.Default().Info("apparmor_parser path resolved", slog.String("path", cfg.ProfilerFullPath))
	}

	// Check if custom directory exists, creates it otherwise
	if _, err := os.Stat(cfg.EtcApparmord); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(cfg.EtcApparmord, rwx_rx_no)
		if err != nil {
			return 0, err
		}

		slog.Default().Info("Directory created", slog.String("path", cfg.EtcApparmord))
	}

	return pollTime, nil
}

// HasTheSameContent compares the byte content of two given files.
// It supports both local filesystem (nil fsys) and virtual fs.FS systems.
func HasTheSameContent(fsys fs.FS, filePath1, filePath2 string) (bool, error) {
	if fsys == nil {
		return compareLocalFiles(filePath1, filePath2)
	}

	return compareFSFiles(fsys, filePath1, filePath2)
}

func compareLocalFiles(filePath1, filePath2 string) (bool, error) {
	if !isSafePath(filePath1) || !isSafePath(filePath2) {
		return false, errors.New("unsafe file path detected")
	}

	fileBytes1, err := os.ReadFile(filePath1) // #nosec G304 -- path validated
	if err != nil {
		slog.Default().Error("read file error", slog.Any("error", err))
		os.Exit(1)
	}

	fileBytes2, err := os.ReadFile(filePath2) // #nosec G304 -- path validated
	if err != nil {
		slog.Default().Error("read file error", slog.Any("error", err))
		os.Exit(1)
	}

	trimmedBytes1 := bytes.TrimSpace(fileBytes1)
	trimmedBytes2 := bytes.TrimSpace(fileBytes2)

	return bytes.Equal(trimmedBytes1, trimmedBytes2), nil
}

func compareFSFiles(fsys fs.FS, filePath1, filePath2 string) (bool, error) {
	dir, err := fs.ReadDir(fsys, ".")
	if err != nil {
		slog.Default().Error("ERROR opening directory", slog.Any("fs", fsys))

		return false, err
	}

	var f1Info, f2Info fs.FileInfo

	for _, file := range dir {
		switch file.Name() {
		case filePath1:
			f1Info, _ = file.Info()
		case filePath2:
			f2Info, _ = file.Info()
		}
	}

	if f1Info == nil || f2Info == nil {
		return false, errors.New("ERROR: files not found")
	}

	if f1Info.Size() != f2Info.Size() {
		return false, nil
	}

	f1, err := openFSFileSafely(fsys, f1Info.Name(), "file1")
	if err != nil {
		return false, err
	}
	defer f1.Close()

	f2, err := openFSFileSafely(fsys, f2Info.Name(), "file2")
	if err != nil {
		return false, err
	}
	defer f2.Close()

	return compareBytes(f1, f2)
}

func openFSFileSafely(fsys fs.FS, name, label string) (fs.File, error) {
	f, err := fsys.Open(name)
	if err != nil {
		slog.Default().Error("open file error", slog.String("label", label), slog.Any("error", err))

		return nil, err
	}

	return f, nil
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

// areProfilesReadable checks if all files in the given folder are readable AppArmor profiles.
func areProfilesReadable(folderName string) (bool, map[string]bool) {
	filenames := map[string]bool{}

	files, err := os.ReadDir(folderName)
	if err != nil {
		slog.Default().Error("readdir error", slog.Any("error", err))
		os.Exit(1)
	}

	if len(files) == 0 {
		slog.Default().Info("No files were found in the given folder")

		return false, nil
	}

	slog.Default().Info("Found files", slog.String("dir", folderName))

	for _, file := range files {
		filename := file.Name()
		if file.IsDir() {
			slog.Default().Info("Directory will be skipped", slog.String("name", filename))

			continue
		} else if strings.HasPrefix(filename, ".") {
			slog.Default().Info("Hidden file will be skipped", slog.String("name", filename))

			continue
		}

		err := IsProfileNameCorrect(folderName, filename)
		if err != nil {
			slog.Default().Error(
				"Found a file issue",
				slog.String("folder", folderName),
				slog.String("filename", filename),
				slog.Any("error", err))
			os.Exit(1)
		}

		slog.Default().Info("profile candidate", slog.String("name", filename))

		filenames[filename] = true
	}

	return true, filenames
}

// IsProfileNameCorrect ensures that the filename matches the AppArmor profile name defined in the file.
func IsProfileNameCorrect(directory, filename string) error {
	// Validate inputs and file presence
	profilePath, err := validateProfileInputs(directory, filename)
	if err != nil {
		return err
	}

	// Ensure syntax has "profile" before "{"
	if err := validateProfileSyntax(profilePath); err != nil {
		return err
	}

	// Extract the declared profile name
	fileProfileName, err := extractProfileName(profilePath)
	if err != nil {
		return err
	}

	// Compare file name and declared profile name
	if filename != fileProfileName {
		return fmt.Errorf("filename '%s' and profile name '%s' seems to be different", filename, fileProfileName)
	}

	return nil
}

// --- helper functions below ---

// validateProfileInputs performs path, filename and existence checks.
func validateProfileInputs(directory, filename string) (string, error) {
	if ok, err := isValidPath(directory); !ok {
		return "", err
	}

	if ok, err := isValidFilename(filename); !ok {
		return "", err
	}

	profilePath := path.Join(directory, filename)
	if !isSafePath(profilePath) {
		return "", fmt.Errorf("unsafe file path detected: %s", profilePath)
	}

	if _, err := os.Stat(profilePath); errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	return profilePath, nil
}

// validateProfileSyntax ensures "profile" appears before the opening '{'.
func validateProfileSyntax(profilePath string) error {
	fileBytes, err := os.ReadFile(profilePath) // #nosec G304 -- validated path
	if err != nil {
		return err
	}

	profileIndex := bytes.Index(fileBytes, []byte("profile"))

	curlyIndex := bytes.Index(fileBytes, []byte("{"))

	if curlyIndex < 0 || curlyIndex < profileIndex {
		return errors.New("couldn't find a { after 'profile' keyword")
	}

	return nil
}

// extractProfileName scans the file and returns the declared profile name.
func extractProfileName(profilePath string) (string, error) {
	const minTokensExpectedInProfileNameLine = 2

	f, err := os.Open(profilePath) // #nosec G304 -- validated path
	if err != nil {
		return "", err
	}

	defer func() {
		cerr := f.Close()
		if cerr != nil {
			slog.Default().Warn("error closing fileReader", slog.Any("error", cerr))
		}
	}()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Only lines that begin with "profile "
		if !strings.HasPrefix(line, "profile ") {
			continue
		}

		tokens := strings.Fields(line)
		if len(tokens) < minTokensExpectedInProfileNameLine {
			continue
		}

		name := strings.TrimSpace(tokens[1])
		slog.Default().Info("Found profile name", slog.String("name", name))

		return name, nil
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scanner error: %w", err)
	}

	return "", errors.New(
		`there is an issue with the profile name!\n
		Please check if the syntax is 'profile custom.yourName { ... }' or consult AppArmor docs`,
	)
}

func isValidPath(path string) (bool, error) {
	if len(path) == 0 {
		return false, errors.New("empty directory name")
	}

	cleanPath := filepath.Clean(path)
	substrings := strings.SplitSeq(cleanPath, string(os.PathSeparator))

	for substring := range substrings {
		// Skip empty substrings (can happen with leading/trailing slashes)
		if len(substring) == 0 {
			continue
		}
		// '.' is a valid path name but not a valid filename
		if len(substring) == 1 && substring[0] == '.' {
			continue
		}

		if ok, err := isValidFilename(substring); !ok {
			return false, err
		}
	}

	return true, nil
}

// isAlphaNumeric checks if a rune is a letter or a digit.
// Extracted from isValidFilename to reduce complexity.
func isAlphaNumeric(char rune) bool {
	return unicode.IsDigit(char) || unicode.IsLetter(char)
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
func isValidFilename(filename string) (bool, error) { //nolint: gocyclo
	// --- Guard Clauses ---
	if len(filename) == 0 {
		return false, errors.New("empty filename")
	}

	if len(filename) == 1 && filename[0] == '.' {
		return false, fmt.Errorf("%q is not a valid filename", filename)
	}

	if len(filename) >= maximumLinuxFilenameLen {
		return false, errors.New("file name too long")
	}

	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return false, errors.New("invalid filename format")
	}

	// --- Character Validation Loop ---
	validSymbols := []rune{'_', '-', '.'}
	var previousCharIsASymbol bool

	for i, char := range filename {
		isSymbol := isCharInSlice(char, validSymbols)
		isAlnum := isAlphaNumeric(char)

		// Check 1: Must be alphanumeric OR a valid symbol
		if !isAlnum && !isSymbol {
			return false, errors.New("invalid characters in filename")
		}

		// Check 2: Reject consecutive symbols (e.g., "--" or ".-")
		if i > 0 && isSymbol && previousCharIsASymbol {
			return false, errors.New("rejected suspect filename")
		}

		// Update state for the next iteration
		previousCharIsASymbol = isSymbol
	}

	return true, nil
}

func isCharInSlice(char rune, slice []rune) bool {
	return slices.Contains(slice, char)
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
		slog.Default().Error("stat src error", slog.Any("error", err))
		os.Exit(1)
	}

	if !sfi.Mode().IsRegular() {
		// cannot copy non-regular files (e.g., directories symlinks, devices, etc.)
		return fmt.Errorf("CopyFile: non-regular source file %s (%q)", sfi.Name(), sfi.Mode().String())
	}

	dfi, err := os.Stat(dstCompleteFileName)
	if err != nil {
		slog.Default().Warn("stat dst error", slog.Any("error", err))
	} else {
		if !(dfi.Mode().IsRegular()) {
			return fmt.Errorf("CopyFile: non-regular destination file %s (%q)", dfi.Name(), dfi.Mode().String())
		}

		if os.SameFile(sfi, dfi) {
			slog.Default().Info("File already present", slog.String("path", dstCompleteFileName))

			return nil
		}
	}

	if err = os.Link(src, dstCompleteFileName); err == nil {
		slog.Default().Info("Hard link created", slog.String("path", dstCompleteFileName))

		return nil
	}

	slog.Default().Info("Copying file", slog.String("src", src), slog.String("dst", dstCompleteFileName))

	return copyFileContents(src, dstCompleteFileName)
}

// copyFileContents copies the contents of the file named src to the file named
// by dst. The file will be created if it does not already exist. If the
// destination file exists, all it's contents will be replaced by the contents
// of the source file.
func copyFileContents(src, dst string) (err error) {
	if !isSafePath(src) || !isSafePath(dst) {
		slog.Default().Warn("unsafe file path detected in copyFileContents")

		return errors.New("unsafe file path detected")
	}

	in, err := os.Open(src) // #nosec G304 -- path validated by isSafePath
	if err != nil {
		slog.Default().Error("open src error", slog.Any("error", err))

		return err
	}

	defer func() {
		err := in.Close()
		if err != nil {
			slog.Default().Warn("error closing input file", slog.Any("error", err))
		}
	}()

	out, err := os.Create(dst) // #nosec G304 -- path validated by isSafePath
	if err != nil {
		slog.Default().Error("create dst error", slog.Any("error", err))

		return err
	}

	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		slog.Default().Error("copy error", slog.Any("error", err))

		return err
	}

	err = out.Sync()

	return err
}
