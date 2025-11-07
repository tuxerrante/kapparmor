package main

import (
	"log/slog"
	"os"
	"path"
	"sync"
)

// Thread-safe lock for file operations.
var profileOperationsMutex sync.Mutex

// AppConfig groups runtime configuration and shared app state.
type AppConfig struct {
	ConfigmapPath     string
	EtcApparmord      string
	PollTimeArg       string
	ProfilerBinFolder string
	ProfilerFullPath  string
	KernelPath        string
	Logger            *slog.Logger

	// Do not use a os.Signals: RunApp() manages signals and context locally.
}

// NewConfigFromEnv initializes AppConfig from environment with secure defaults.
func NewConfigFromEnv(logger *slog.Logger) *AppConfig {
	configmapPath := os.Getenv("PROFILES_DIR")
	if configmapPath == "" {
		configmapPath = "/app/profiles"
	}

	pollTimeArg := os.Getenv("POLL_TIME")
	if pollTimeArg == "" {
		pollTimeArg = "30"
	}

	profilerBinFolder := "/sbin"
	profilerFullPath := path.Join(profilerBinFolder, ProfilerBin)

	config := &AppConfig{
		ConfigmapPath:     configmapPath,
		EtcApparmord:      "/etc/apparmor.d/custom",
		PollTimeArg:       pollTimeArg,
		ProfilerBinFolder: profilerBinFolder,
		ProfilerFullPath:  profilerFullPath,
		KernelPath:        "/sys/kernel/security/apparmor/profiles",
		Logger:            logger,
	}

	logger.Info("Configuration initialized",
		slog.String("profiles_dir", config.ConfigmapPath),
		slog.String("etc_apparmord", config.EtcApparmord),
		slog.String("poll_time", config.PollTimeArg),
		slog.String("profiler_path", config.ProfilerFullPath),
		slog.String("kernel_path", config.KernelPath),
	)

	return config
}
