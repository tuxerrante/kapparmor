package main

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func startHealthzServer(cfg *AppConfig) {
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	http.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		_, desired := getNewProfiles(cfg)
		_, loaded, _ := getLoadedProfiles(cfg)

		inSync := true

		if len(desired) == len(loaded) {
			for profile := range desired {
				if !loaded[profile] {
					inSync = false

					break
				}
			}

			if inSync {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("READY"))
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("NOT_READY"))
			}
		}
	})

	http.Handle("/metrics", promhttp.Handler())

	go func() {
		slog.Default().Info("Starting healthz server",
			slog.Int("port", HealthzPort),
			slog.String("health_endpoint", "/healthz"),
			slog.String("ready_endpoint", "/readyz"),
			slog.String("metrics_endpoint", "/metrics"),
		)

		if err := http.ListenAndServe(fmt.Sprintf(":%d", HealthzPort), nil); err != nil {
			slog.Default().Error("Healthz server failed", slog.Any("error", err))
		}
	}()
}
