package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"strconv"
)

func preFlightChecks() int {

	// Environment variable type check
	POLL_TIME, err := strconv.Atoi(POLL_TIME_ARG)
	if err != nil {
		log.Fatalf(">> It was not possible to convert env var POLL_TIME %v to an integer.\n%v", POLL_TIME, err)
	}
	log.Printf("POLL_TIME set to %d sec.", POLL_TIME)

	// Check profiler binary
	if _, err := os.Stat(PROFILER_BIN); os.IsNotExist(err) {
		log.Fatal(err)
	}

	// Check if custom directory exists
	if _, err := os.Stat(ETC_APPARMORD); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(ETC_APPARMORD, os.ModePerm)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("> Directory %s created.", ETC_APPARMORD)
	}

	return POLL_TIME
}

func HasTheSameContent(fsys fs.FS, filePath1, filePath2 string) (bool, error) {

	var file1, file2 os.FileInfo
	dir, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return false, err
	}

	for _, file := range dir {
		if filePath1 == file.Name() {
			file1, _ = file.Info()
		} else if filePath2 == file.Name() {
			file2, _ = file.Info()
		}
	}

	if file1 == nil || file2 == nil {
		return false, fmt.Errorf("files not found")
	}

	if file1.Size() != file2.Size() {
		return false, nil
	}

	f1, err := fsys.Open(file1.Name())
	if err != nil {
		return false, err
	}
	defer f1.Close()

	data1, err := io.ReadAll(f1)
	if err != nil {
		return false, err
	}

	f2, err := fsys.Open(file2.Name())
	if err != nil {
		return false, err
	}
	defer f2.Close()

	data2, err := io.ReadAll(f2)
	if err != nil {
		return false, err
	}

	if !bytes.Equal(data1, data2) {
		return false, nil
	}

	return true, nil
}

func areProfilesReadable(FOLDER_NAME string) (bool, map[string]bool) {

	filenames := map[string]bool{}
	files, err := os.ReadDir(FOLDER_NAME)
	if err != nil {
		log.Fatal(err.Error())
	}

	if len(files) == 0 {
		log.Printf("No files were found in the given folder!\n")
		return false, nil
	}

	log.Printf("Found files in given folder:\n")
	for _, file := range files {
		if file.IsDir() {
			log.Printf("Directory '%s' will be skipped.\n", file.Name())
			continue
		}
		log.Printf("- %s\n", file.Name())
		filenames[file.Name()] = true
	}

	return true, filenames
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
