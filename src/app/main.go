package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

func main() {
	logger := newDefaultLogger()
	cfg := NewConfigFromEnv(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startHealthzServer(cfg)

	if err := RunApp(ctx, cfg); err != nil {
		logger.Error("application error", slog.Any("error", err))
		os.Exit(1)
	}
}

// RunApp starts the poller and handles graceful shutdown.
// It blocks until a stop signal is received or ctx is canceled.
func RunApp(parentCtx context.Context, cfg *AppConfig) error {
	const contextTimeout = 20
	slog.SetDefault(cfg.Logger)
	pollTime, err := preFlightChecks(cfg)
	if err != nil {
		cfg.Logger.Error("the app can't start",
			slog.Any("error", err),
			slog.String("POLL_TIME", cfg.PollTimeArg),
			slog.String("ETC_APPARMORD", cfg.EtcApparmord),
			slog.String("apparmor_parser_path", cfg.ProfilerFullPath),
		)

		return err
	}
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	cfg.Logger.Info("Polling directory",
		slog.String("dir", cfg.ConfigmapPath),
		slog.Int("seconds", pollTime))

	// Use WaitGroup to track goroutine completion and start polling.
	var wg sync.WaitGroup
	wg.Go(func() {
		pollProfiles(ctx, cfg, pollTime)
	})

	// Separate signal handling - no shared channel confusion
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT, os.Interrupt)

	// Wait for shutdown signal
	select {
	case sig := <-sigChan:
		cfg.Logger.Info("Received stop signal, terminating..", slog.String("signal", sig.String()))
	case <-parentCtx.Done():
		cfg.Logger.Info("Parent context canceled")
	}

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), contextTimeout*time.Second)
	defer shutdownCancel()

	// Stop polling first
	cancel()

	// Wait for poller to finish with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		cfg.Logger.Info("Poller stopped gracefully")
	case <-shutdownCtx.Done():
		cfg.Logger.Warn("Poller shutdown timeout exceeded")
	}

	if err := unloadAllProfiles(cfg); err != nil {
		cfg.Logger.Error("failed to unload all profiles during shutdown", slog.Any("error", err))
		// Don't return error - attempt best-effort cleanup
	}

	cfg.Logger.Info("The eagle has landed. Over and out.")

	return nil
}

// Every pollTime seconds it will read the mounted volume for profiles,
// it will call loadNewProfiles() then to check if they are new ones or not.
// Executed as go-routine it will run forever until a cancel() is called on the given context.
func pollProfiles(ctx context.Context, cfg *AppConfig, pollTime int) {
	slog.Default().Info("Polling started.")

	if os.Getenv("TESTING") == "true" { //nolint:nestif
		defer func() {
			if r := recover(); r != nil {
				if r, ok := r.(error); ok {
					slog.Default().Warn("panic during test", slog.String("error", r.Error()))

					if strings.Contains(r.Error(), "You need root privileges") {
						slog.Default().Warn("Recovered panic for missing privileges during tests", slog.String("error", r.Error()))

						ctx.Done()
					} else if strings.Contains(r.Error(), "executable file not found") {
						slog.Default().Warn("Recovered panic for missing apparmor binary during tests", slog.String("error", r.Error()))

						ctx.Done()
					}
				}
			}
		}()
	}

	ticker := time.NewTicker(time.Duration(pollTime) * time.Second)
	defer ticker.Stop()

	pollNow := func() {
		// Wrap in recover to prevent single poll failure from killing poller
		defer func() {
			if r := recover(); r != nil {
				slog.Default().Error("panic during profile polling", slog.Any("panic", r))
			}
		}()

		newProfiles, err := loadNewProfiles(cfg)
		slog.Default().Info("retrieving profiles", slog.Any("profiles", newProfiles))
		if err != nil {
			slog.Default().Warn("Failed to load/unload profiles this cycle", slog.Any("error", err))

			return
		}
	}

	for {
		select {
		case <-ctx.Done():
			slog.Default().Info("Polling stopped by context cancellation")

			return
		case <-ticker.C:
			pollNow()
		}
	}
}

// printLoadedProfiles prints node apparmor loaded profiles.
func printLoadedProfiles(p map[string]bool) {
	delete(p, "")

	// Sort alphabetically the profiles and print them
	slog.Default().Info("Profiles already on this node", slog.Int("count", len(p)))
	loadedProfileNames := make([]string, len(p))
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

// showProfilesDiff shows the difference original and current profiles.
func showProfilesDiff(cfg *AppConfig, filePath1, newProfileName string) {
	slog.Default().Warn("Content changed, logging diff...", slog.String("name", newProfileName))
	fileBytes1, _ := os.ReadFile(filePath1)
	fileBytes2, _ := os.ReadFile(path.Join(cfg.EtcApparmord, newProfileName))
	slog.Default().Warn("--- SOURCE FILE ---")
	slog.Default().Warn(string(fileBytes1))
	slog.Default().Warn("--- DEST FILE   ---")
	slog.Default().Warn(string(fileBytes2))
	slog.Default().Warn("--- END DIFF    ---")
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

			contentIsTheSame, err := HasTheSameContent(nil, filePath1, path.Join(cfg.EtcApparmord, newProfileName))
			if err != nil {
				// Error checking file contents
				return nil, nil, fmt.Errorf("error checking content of %q vs %q: %w", filePath1, newProfileName, err)
			}

			if contentIsTheSame {
				slog.Default().Info("Contents are the same, skipping", slog.String("name", newProfileName))

				continue
			}
			slog.Default().Info("Content changed, scheduling replacement", slog.String("name", newProfileName))
			showProfilesDiff(cfg, filePath1, newProfileName)
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

// Check if the current profiles are really new and loads them after verifying some conditions.
func loadNewProfiles(cfg *AppConfig) ([]string, error) {
	profileOperationsMutex.Lock()
	defer profileOperationsMutex.Unlock()

	// 1. Get desired state from ConfigMap
	profilesAreReadable, newProfiles := getNewProfiles(cfg)
	if !profilesAreReadable {
		return nil, fmt.Errorf("error accessing the files in %s", cfg.ConfigmapPath)
	}

	// 2. Get current state from the node
	// 	`loadedProfiles` contains all the profiles loaded in the kernel
	// 	`customLoadedProfiles` contains only the profiles loaded from our EtcApparmord folder
	loadedProfiles, customLoadedProfiles, err := getLoadedProfiles(cfg)
	if err != nil {
		return nil, fmt.Errorf("error reading existing profiles: %w", err)
	}
	delete(customLoadedProfiles, "")

	if os.Getenv("TESTING") == "true" {
		printLoadedProfiles(loadedProfiles)
	}

	// 3. DIFF desired VS current state
	newProfilesToApply, loadedProfilesToUnload, err := calculateProfileChanges(cfg, newProfiles, customLoadedProfiles)
	if err != nil {
		return nil, fmt.Errorf("error calculating profile changes: %w", err)
	}

	// 4. Execute apparmor_parser --replace
	printLogSeparator()
	slog.Default().Info("Apparmor REPLACE and apply new profiles..")

	// Collect errors.
	var applyErrors []error
	for _, profilePath := range newProfilesToApply {
		if err := loadProfile(cfg, profilePath); err != nil {
			slog.Default().Error("apply profile error", slog.Any("error", err))
			applyErrors = append(applyErrors, err)
		}
	}

	// 5. Execute apparmor_parser --remove
	if len(loadedProfilesToUnload) > 0 {
		printLogSeparator()
		slog.Default().Info("AppArmor REMOVE orphans profiles..")

		for _, profileFileName := range loadedProfilesToUnload {
			if err := unloadProfile(cfg, profileFileName); err != nil {
				slog.Default().Error("remove orphan profile error", slog.Any("error", err))
				applyErrors = append(applyErrors, err)
			}
		}
	}

	slog.Default().Info("> Done! > Waiting next poll..")
	printLogSeparator()

	if len(applyErrors) > 0 {
		return newProfilesToApply, fmt.Errorf("encountered %d errors during profile operations", len(applyErrors))
	}

	return newProfilesToApply, nil
}

// It reads the files provided in the ConfigmapPath.
func getNewProfiles(cfg *AppConfig) (bool, map[string]bool) {
	return areProfilesReadable(cfg.ConfigmapPath)
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

// Load an apparmor profile into the kernel.
func loadProfile(cfg *AppConfig, profilePath string) error {
	if err := execApparmor(cfg, "--verbose", "--replace", profilePath); err != nil {
		return fmt.Errorf("failed to load profile into kernel: %w", err)
	}

	slog.Default().Info("Copying profile", slog.String("dest", cfg.EtcApparmord))

	if err := CopyFile(profilePath, cfg.EtcApparmord); err != nil {
		return fmt.Errorf("failed to copy profile to destination: %w", err)
	}

	return nil
}

// Remove all custom profiles from the kernel, reading from ETC_APPARMORD folder.
func unloadAllProfiles(cfg *AppConfig) error {
	slog.Default().Info("Unloading all custom profiles from kernel and filesystem...")
	dirEntries, err := os.ReadDir(cfg.EtcApparmord)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			slog.Default().Warn("Custom profile directory does not exist, nothing to unload",
				slog.String("path", cfg.EtcApparmord))

			return nil // Nothing to do
		}
		slog.Default().Error(
			"Cannot read custom profile directory",
			slog.String("path", cfg.EtcApparmord),
			slog.Any("error", err),
		)

		return err // Return the error, don't panic
	}

	var errs []error
	for _, entry := range dirEntries {
		if !entry.IsDir() && entry.Type().IsRegular() {
			if err := unloadProfile(cfg, entry.Name()); err != nil {
				slog.Default().Error("failed to unload profile",
					slog.String("profile", entry.Name()),
					slog.Any("error", err))
				errs = append(errs, err)
				// Continue with other profiles instead of stopping
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to unload %d profile(s): %w", len(errs), errors.Join(errs...))
	}

	return nil
}

// Remove an apparmor profile from the kernel.
func unloadProfile(cfg *AppConfig, fileName string) error {
	// Use path.Base for security, consistent with fuzz test fix
	safeFileName := path.Base(fileName)
	filePath := path.Join(cfg.EtcApparmord, safeFileName)

	// Check if the file exists first.
	if _, err := os.Stat(filePath); errors.Is(os.ErrNotExist, err) {
		slog.Default().Info("Profile file does not exist, skipping unload", slog.String("profile", filePath))

		return nil // Nothing to do
	}

	var errs []error

	// 1. Try to remove from kernel first
	if err := execApparmor(cfg, "--verbose", "--remove", filePath); err != nil {
		// Log the error but don't panic or stop.
		// It might fail if the profile isn't loaded, which is fine during cleanup.
		slog.Default().Warn("failed to remove profile from kernel (might be expected on cleanup)",
			slog.String("profile", filePath),
			slog.Any("error", err))
		errs = append(errs, fmt.Errorf("parser removal: %w", err))
	}

	// 2. Now try to remove the file, even if kernel removal failed
	if err := os.Remove(filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		// Log any error *except* "not found".
		// If it's not found, that's fine.
		slog.Default().Error("failed to remove profile file from disk",
			slog.String("profile", filePath),
			slog.Any("error", err))
		errs = append(errs, fmt.Errorf("file removal: %w", err))

		return errors.Join(errs...) // Return the filesystem error
	}

	// 3. Reload AppArmor to ensure it picks up the changes
	if len(errs) == 0 {
		if err := execApparmor(cfg, "--reload", cfg.EtcApparmord); err != nil {
			slog.Default().Warn("failed to reload AppArmor after profile removal",
				slog.String("profile", filePath),
				slog.Any("error", err))
			errs = append(errs, fmt.Errorf("apparmor reload: %w", err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	// If we get here, it either worked, or the errors were expected (not found)
	slog.Default().Info("Successfully unloaded and removed profile", slog.String("profile", filePath))

	return nil
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
