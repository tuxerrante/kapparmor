// Package metrics provides info related to CRUD operations over Apparmor profiles
package metrics

import (
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	nodeName = getNodeNameFromEnv()

	// profileOperations counts create/modify/delete operations per profile.
	profileOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   "kapparmor",
			Name:        "profile_operations_total",
			Help:        "Numero totale di operazioni sui profili (create, modify, delete).",
			ConstLabels: prometheus.Labels{"node_name": nodeName},
		},
		[]string{"operation", "profile_name"},
	)

	// currentProfiles tracks how many profiles are currently managed.
	currentProfiles = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace:   "kapparmor",
		Name:        "profiles_managed",
		Help:        "Numero totale di profili AppArmor attualmente gestiti.",
		ConstLabels: prometheus.Labels{"node_name": nodeName},
	})
)

func getNodeNameFromEnv() string {
	if n := os.Getenv("NODE_NAME"); n != "" {
		return n
	}
	if hn, err := os.Hostname(); err == nil {
		return hn
	}

	return "unknown"
}

// Metrics setters

// ProfileCreated increments create counter and increments gauge.
func ProfileCreated(p string) {
	profileOperations.WithLabelValues("create", p).Inc()
	currentProfiles.Inc()
}

// ProfileDeleted increments delete counter and decrements gauge.
func ProfileDeleted(p string) {
	profileOperations.WithLabelValues("delete", p).Inc()
	currentProfiles.Dec()
}

// ProfileModified is an alias used by tests for updates.
func ProfileModified(p string) {
	profileOperations.WithLabelValues("modify", p).Inc()
}

// ProfileUpdated kept for compatibility.
func ProfileUpdated(p string) {
	ProfileModified(p)
}

// SetProfileCount sets the gauge to c.
func SetProfileCount(c int) {
	currentProfiles.Set(float64(c))
}
