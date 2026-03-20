package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"

	"github.com/tuxerrante/kapparmor/src/app/metrics"
)

// printLoadedProfiles prints node apparmor loaded profiles.
func printLoadedProfiles(p map[string]bool) {
	delete(p, "")

	// Sort alphabetically the profiles and print them
	slog.Default().Info("Profiles already on this node", slog.Int("count", len(p)))
	loadedProfileNames := make([]string, 0, len(p))
	for loadedProfileName := range p {
		loadedProfileNames = append(loadedProfileNames, loadedProfileName)
	}

	sort.Strings(loadedProfileNames)
	for _, p := range loadedProfileNames {
		if p != "" {
			slog.Default().Info("profile", slog.String("name", p))
		}
	}
}

// showProfilesDiff logs metadata about changed profiles without exposing full content.
// Full content is redacted to prevent information disclosure (threat T7).
func showProfilesDiff(cfg *AppConfig, newProfileName string) {
	srcBytes, srcErr := readProfileBytes(cfg.ConfigmapRoot, cfg.ConfigmapPath, newProfileName)
	dstBytes, dstErr := readProfileBytes(cfg.EtcRoot, cfg.EtcApparmord, newProfileName)

	srcHash, srcLines := profileDigest(srcBytes, srcErr)
	dstHash, dstLines := profileDigest(dstBytes, dstErr)

	attrs := []any{
		slog.String("name", newProfileName),
		slog.String("src_sha256", srcHash),
		slog.Int("src_size", len(srcBytes)),
		slog.Int("src_lines", srcLines),
		slog.String("dst_sha256", dstHash),
		slog.Int("dst_size", len(dstBytes)),
		slog.Int("dst_lines", dstLines),
	}
	if srcErr != nil {
		attrs = append(attrs, slog.String("src_error", srcErr.Error()))
	}
	if dstErr != nil {
		attrs = append(attrs, slog.String("dst_error", dstErr.Error()))
	}

	slog.Default().Warn("Profile content changed", attrs...)
}

func profileDigest(data []byte, readErr error) (hash string, lines int) {
	if readErr != nil {
		return "<read-error>", 0
	}
	h := sha256.Sum256(data)

	return fmt.Sprintf("%x", h[:]), countLines(data)
}

// countLines counts the number of lines in data using wc -l semantics:
// returns 0 for empty input, counts newlines, and adds 1 if the last
// line does not end with a newline character.
func countLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	n := bytes.Count(data, []byte("\n"))
	if !bytes.HasSuffix(data, []byte("\n")) {
		n++
	}

	return n
}

// calculateProfileChanges compares desired state (newProfiles) vs current state (customLoadedProfiles).
// It returns two lists: profiles to apply and profiles to unload/remove.
func calculateProfileChanges(cfg *AppConfig, newProfiles map[string]bool, customLoadedProfiles map[string]bool) (
	toApply []string,
	toUnload []string,
	err error,
) {
	newProfilesToApply := make([]string, 0, len(newProfiles))

	for newProfileName := range newProfiles {
		filePath1 := path.Join(cfg.ConfigmapPath, newProfileName)

		// Does it exist a profile with the same name already loaded?
		if customLoadedProfiles[newProfileName] {
			slog.Default().Info("Checking profile", slog.String("path", filePath1))

			srcBytes, errSrc := readProfileBytes(cfg.ConfigmapRoot, cfg.ConfigmapPath, newProfileName)
			dstBytes, errDst := readProfileBytes(cfg.EtcRoot, cfg.EtcApparmord, newProfileName)
			if errSrc != nil || errDst != nil {
				return nil, nil, fmt.Errorf("error checking content of profile %q: configmap: %v; etc: %v",
					newProfileName, errSrc, errDst)
			}

			if profileBytesEqual(srcBytes, dstBytes) {
				slog.Default().Info("Contents are the same, skipping", slog.String("name", newProfileName))

				continue
			}
			slog.Default().Info("Content changed, scheduling replacement", slog.String("name", newProfileName))
			showProfilesDiff(cfg, newProfileName)
			metrics.ProfileModified(newProfileName)
		} else {
			slog.Default().Info("New profile found, scheduling for load", slog.String("name", newProfileName))
		}

		newProfilesToApply = append(newProfilesToApply, filePath1)
	}

	loadedProfilesToUnload := make([]string, 0, len(customLoadedProfiles))

	for customLoadedProfile := range customLoadedProfiles {
		if !newProfiles[customLoadedProfile] {
			loadedProfilesToUnload = append(loadedProfilesToUnload, customLoadedProfile)
		}
	}

	return newProfilesToApply, loadedProfilesToUnload, nil
}

// It reads the files provided in the ConfigmapPath.
func getNewProfiles(cfg *AppConfig) (bool, map[string]bool) {
	return areProfilesReadable(cfg)
}

// It reads a list of profile names from a singe file under KERNEL_PATH.
func getLoadedProfiles(cfg *AppConfig) (map[string]bool, map[string]bool, error) {
	return getProfilesNamesFromFile(cfg.KernelPath, ProfileNamePrefix)
}

// Search for profiles already present on the current node in '$apparmorfs/profiles' folder
// It returns two maps to split the custom profiles introduced by us and the built-ins in the node OS
// Output
//   - profiles{} map containing all the loaded profiles
//   - customProfiles{} map containing only the profiles starting with the given PREFIX
func getProfilesNamesFromFile(profilesPath, profileNamePrefix string) (map[string]bool, map[string]bool, error) {
	profilesFile, err := os.Open(profilesPath) // #nosec G304 -- profilesPath is a system path
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open %s: %w", profilesPath, err)
	}

	defer func() {
		err := profilesFile.Close()
		if err != nil {
			slog.Default().Warn("error closing profilesFile", slog.Any("error", err))
		}
	}()

	profiles := map[string]bool{}
	customProfiles := map[string]bool{}

	scanner := bufio.NewScanner(profilesFile)

	for scanner.Scan() {
		profileName := parseProfileName(scanner.Text())
		if profileName == "" {
			continue
		}

		if strings.HasPrefix(profileName, profileNamePrefix) {
			customProfiles[profileName] = true
		}

		profiles[profileName] = true
	}

	return profiles, customProfiles, nil
}

func parseProfileName(profileLine string) string {
	modeIndex := strings.IndexRune(profileLine, '(')
	if modeIndex < 0 {
		return ""
	}

	return strings.TrimSpace(profileLine[:modeIndex])
}

func execApparmor(cfg *AppConfig, args ...string) error {
	cmd := exec.Command(cfg.ProfilerFullPath, args...) // #nosec G204 -- profilename validated before
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	out, err := cmd.Output()

	path := args[len(args)-1]

	if len(out) > 0 {
		slog.Default().Info("execApparmor", slog.String("path", path), slog.String("stdout", string(out)))
	} else {
		slog.Default().Info("No profiles", slog.String("path", path))
	}

	if err != nil {
		if stderr.Len() > 0 {
			slog.Default().Error("apparmor_parser stderr", slog.String("stderr", stderr.String()))
		}

		return fmt.Errorf("error loading profile >> %w >> %v", err, stderr)
	}

	return nil
}

// A line separator to simplify logs reading.
func printLogSeparator() {
	slog.Default().Info("============================================================")
}
