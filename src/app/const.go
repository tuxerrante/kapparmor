package main

const (
	MaxAllowedPollingTime   = 86400 // 24 hours
	ProfilerBin             = "apparmor_parser"
	ProfileNamePrefix       = "custom."
	maximumLinuxFilenameLen = 255
	rwx_rx_no               = 0o750
	HealthzPort             = 8080
)
