package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/tuxerrante/kapparmor/src/app/metrics"
)

func writeProfileFile(t *testing.T, dir, name, body string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write profile %s: %v", name, err)
	}
}

func writeKernelProfiles(t *testing.T, kernelPath string, names ...string) {
	t.Helper()

	var builder strings.Builder
	for _, name := range names {
		builder.WriteString(name)
		builder.WriteString(" (enforce)\n")
	}

	if err := os.WriteFile(kernelPath, []byte(builder.String()), 0o600); err != nil {
		t.Fatalf("write kernel profiles: %v", err)
	}
}

func writeKernelUpdatingParser(t *testing.T, cfg *AppConfig, failingProfiles ...string) string {
	t.Helper()

	var failCases strings.Builder
	for _, profile := range failingProfiles {
		failCases.WriteString("\t\t")
		failCases.WriteString(profile)
		failCases.WriteString(")\n")
		failCases.WriteString("\t\t\techo 'ERR: simulated failure' 1>&2\n")
		failCases.WriteString("\t\t\texit 1\n")
		failCases.WriteString("\t\t\t;;\n")
	}
	failCases.WriteString("\t\t*) ;;\n")

	scriptPath := filepath.Join(filepath.Dir(cfg.ProfilerFullPath), "apparmor_parser")
	script := fmt.Sprintf(`#!/bin/sh
set -eu

kernel=%q
last_arg=""
for arg in "$@"; do
	last_arg="$arg"
done

name=$(basename "$last_arg")
line="$name (enforce)"
tmp="$kernel.tmp"

case " $* " in
  *" --replace "*)
    case "$name" in
%s
    esac
    if [ -f "$kernel" ]; then
      awk -v name="$name" '$1 != name { print }' "$kernel" > "$tmp"
    else
      : > "$tmp"
    fi
    printf '%%s\n' "$line" >> "$tmp"
    mv "$tmp" "$kernel"
    echo 'OK: simulated load'
    ;;
  *" --remove "*)
    if [ -f "$kernel" ]; then
      awk -v name="$name" '$1 != name { print }' "$kernel" > "$tmp"
      mv "$tmp" "$kernel"
    fi
    echo 'OK: simulated remove'
    ;;
  *" --reload "*)
    echo 'OK: simulated reload'
    ;;
  *)
    echo 'OK: simulated noop'
    ;;
esac
`, cfg.KernelPath, failCases.String())

	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write parser script: %v", err)
	}

	return scriptPath
}

func managedProfilesGaugeValue(t *testing.T) float64 {
	t.Helper()

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	for _, family := range families {
		if family.GetName() != "kapparmor_profiles_managed" {
			continue
		}
		if len(family.GetMetric()) != 1 {
			t.Fatalf("expected one managed profile gauge, got %d", len(family.GetMetric()))
		}

		return family.GetMetric()[0].GetGauge().GetValue()
	}

	t.Fatal("kapparmor_profiles_managed gauge not found")

	return 0
}

func profileOperationCounterValue(t *testing.T, operation, profileName string) float64 {
	t.Helper()

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	for _, family := range families {
		if family.GetName() != "kapparmor_profile_operations_total" {
			continue
		}

		for _, metric := range family.GetMetric() {
			var gotOperation, gotProfileName string
			for _, label := range metric.GetLabel() {
				switch label.GetName() {
				case "operation":
					gotOperation = label.GetValue()
				case "profile_name":
					gotProfileName = label.GetValue()
				}
			}

			if gotOperation == operation && gotProfileName == profileName {
				return metric.GetCounter().GetValue()
			}
		}
	}

	return 0
}

func TestLoadNewProfilesCountsAlreadyLoadedProfiles(t *testing.T) {
	cfg, _ := preFlightChecksInit(t)
	cfg.ProfilerFullPath = writeKernelUpdatingParser(t, cfg)
	testOpenProfileRoots(t, cfg)

	const profileName = "custom.metricsunchanged"
	content := "profile custom.metricsunchanged { }\n"
	writeProfileFile(t, cfg.ConfigmapPath, profileName, content)
	writeProfileFile(t, cfg.EtcApparmord, profileName, content)
	writeKernelProfiles(t, cfg.KernelPath, profileName)
	metrics.SetProfileCount(0)

	if _, err := loadNewProfiles(cfg); err != nil {
		t.Fatalf("loadNewProfiles: %v", err)
	}

	if got := managedProfilesGaugeValue(t); got != 1 {
		t.Fatalf("expected managed gauge to be 1, got %.0f", got)
	}

	if got := profileOperationCounterValue(t, "create", profileName); got != 0 {
		t.Fatalf("expected create counter to stay at 0, got %.0f", got)
	}
}

func TestLoadNewProfilesDoesNotCountModifiedProfilesAsCreate(t *testing.T) {
	cfg, _ := preFlightChecksInit(t)
	cfg.ProfilerFullPath = writeKernelUpdatingParser(t, cfg)
	testOpenProfileRoots(t, cfg)

	const profileName = "custom.metricsmodified"
	writeProfileFile(t, cfg.ConfigmapPath, profileName, "profile custom.metricsmodified { # desired }\n")
	writeProfileFile(t, cfg.EtcApparmord, profileName, "profile custom.metricsmodified { # current }\n")
	writeKernelProfiles(t, cfg.KernelPath, profileName)
	metrics.SetProfileCount(0)

	if _, err := loadNewProfiles(cfg); err != nil {
		t.Fatalf("loadNewProfiles: %v", err)
	}

	if got := managedProfilesGaugeValue(t); got != 1 {
		t.Fatalf("expected managed gauge to stay at 1, got %.0f", got)
	}

	if got := profileOperationCounterValue(t, "modify", profileName); got != 1 {
		t.Fatalf("expected modify counter to be 1, got %.0f", got)
	}

	if got := profileOperationCounterValue(t, "create", profileName); got != 0 {
		t.Fatalf("expected create counter to stay at 0 for modified profile, got %.0f", got)
	}
}

func TestLoadNewProfilesUpdatesGaugeAfterDeletingOrphans(t *testing.T) {
	cfg, _ := preFlightChecksInit(t)
	cfg.ProfilerFullPath = writeKernelUpdatingParser(t, cfg)
	testOpenProfileRoots(t, cfg)

	const keepProfile = "custom.metricskeep"
	const deleteProfile = "custom.metricsdelete"
	content := "profile %s { }\n"
	writeProfileFile(t, cfg.ConfigmapPath, keepProfile, fmt.Sprintf(content, keepProfile))
	writeProfileFile(t, cfg.EtcApparmord, keepProfile, fmt.Sprintf(content, keepProfile))
	writeProfileFile(t, cfg.EtcApparmord, deleteProfile, fmt.Sprintf(content, deleteProfile))
	writeKernelProfiles(t, cfg.KernelPath, keepProfile, deleteProfile)
	metrics.SetProfileCount(0)

	if _, err := loadNewProfiles(cfg); err != nil {
		t.Fatalf("loadNewProfiles: %v", err)
	}

	if got := managedProfilesGaugeValue(t); got != 1 {
		t.Fatalf("expected managed gauge to be 1 after deleting orphan, got %.0f", got)
	}

	if got := profileOperationCounterValue(t, "delete", deleteProfile); got != 1 {
		t.Fatalf("expected delete counter to be 1, got %.0f", got)
	}
}

func TestLoadNewProfilesPublishesGaugeAfterPartialFailure(t *testing.T) {
	cfg, _ := preFlightChecksInit(t)
	cfg.ProfilerFullPath = writeKernelUpdatingParser(t, cfg, "custom.metricsfail")
	testOpenProfileRoots(t, cfg)

	const okProfile = "custom.metricsok"
	const failProfile = "custom.metricsfail"
	writeProfileFile(t, cfg.ConfigmapPath, okProfile, "profile custom.metricsok { }\n")
	writeProfileFile(t, cfg.ConfigmapPath, failProfile, "profile custom.metricsfail { }\n")
	writeKernelProfiles(t, cfg.KernelPath)
	metrics.SetProfileCount(0)

	if _, err := loadNewProfiles(cfg); err == nil {
		t.Fatal("expected loadNewProfiles to report partial failure")
	}

	if got := managedProfilesGaugeValue(t); got != 1 {
		t.Fatalf("expected managed gauge to count only successful loads, got %.0f", got)
	}

	if got := profileOperationCounterValue(t, "create", okProfile); got != 1 {
		t.Fatalf("expected create counter for successful profile to be 1, got %.0f", got)
	}

	if got := profileOperationCounterValue(t, "create", failProfile); got != 0 {
		t.Fatalf("expected create counter for failed profile to stay at 0, got %.0f", got)
	}
}

func TestLoadNewProfilesIgnoresManagedGaugePublishFailure(t *testing.T) {
	cfg, _ := preFlightChecksInit(t)
	cfg.ProfilerFullPath = writeKernelUpdatingParser(t, cfg)

	const orphanProfile = "custom.metricsorphan"
	if err := os.WriteFile(filepath.Join(cfg.ConfigmapPath, ".ignored"), []byte("ignored"), 0o600); err != nil {
		t.Fatalf("write hidden file: %v", err)
	}

	// Point the kernel profile list at the same file that orphan cleanup removes so
	// publishManagedProfileCount fails only after the unload work has completed.
	cfg.KernelPath = filepath.Join(cfg.EtcApparmord, orphanProfile)
	writeKernelProfiles(t, cfg.KernelPath, orphanProfile)
	metrics.SetProfileCount(9)

	appliedProfiles, err := loadNewProfiles(cfg)
	if err != nil {
		t.Fatalf("loadNewProfiles should ignore managed gauge publish failures: %v", err)
	}

	if len(appliedProfiles) != 0 {
		t.Fatalf("expected no profiles to apply, got %v", appliedProfiles)
	}

	if got := managedProfilesGaugeValue(t); got != 9 {
		t.Fatalf("expected managed gauge to stay at the last published value, got %.0f", got)
	}

	if got := profileOperationCounterValue(t, "delete", orphanProfile); got != 1 {
		t.Fatalf("expected delete counter to be 1, got %.0f", got)
	}

	if _, err := os.Stat(filepath.Join(cfg.EtcApparmord, orphanProfile)); !os.IsNotExist(err) {
		t.Fatalf("expected orphan profile to be removed from disk, got err=%v", err)
	}
}

func TestLoadProfileRollsBackKernelLoadWhenCopyFails(t *testing.T) {
	cfg, _ := preFlightChecksInit(t)
	cfg.ProfilerFullPath = writeKernelUpdatingParser(t, cfg)

	const profileName = "custom.metricsrollback"
	profilePath := filepath.Join(cfg.ConfigmapPath, profileName)
	writeProfileFile(t, cfg.ConfigmapPath, profileName, fmt.Sprintf("profile %s { }\n", profileName))

	badDestination := filepath.Join(filepath.Dir(cfg.EtcApparmord), "not-a-directory")
	if err := os.WriteFile(badDestination, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write bad destination marker: %v", err)
	}
	cfg.EtcApparmord = badDestination

	err := loadProfile(cfg, profilePath)
	if err == nil {
		t.Fatal("expected loadProfile to fail when profile persistence fails")
	}

	if !strings.Contains(err.Error(), "failed to copy profile to destination") {
		t.Fatalf("expected copy failure error, got %v", err)
	}

	_, customLoadedProfiles, err := getLoadedProfiles(cfg)
	if err != nil {
		t.Fatalf("getLoadedProfiles: %v", err)
	}

	if customLoadedProfiles[profileName] {
		t.Fatalf("expected %s to be removed from kernel after rollback", profileName)
	}
}
