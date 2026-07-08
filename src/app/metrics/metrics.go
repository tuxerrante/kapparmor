// Package metrics provides info related to CRUD operations over Apparmor profiles
package metrics

import (
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	nodeName             = getNodeNameFromEnv()
	defaultProfileMetric = newProfileMetrics()

	// These aliases keep the existing test helpers and package-level API intact.
	profileOperations, currentProfiles = aliasesFor(defaultProfileMetric)
)

// ProfileMetrics owns the Prometheus counters and gauges published by the app.
type ProfileMetrics struct {
	profileOperations *prometheus.CounterVec
	managedProfiles   prometheus.Gauge
}

func aliasesFor(m *ProfileMetrics) (*prometheus.CounterVec, prometheus.Gauge) {
	return m.profileOperations, m.managedProfiles
}

func newProfileMetrics() *ProfileMetrics {
	return &ProfileMetrics{
		profileOperations: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   "kapparmor",
				Name:        "profile_operations_total",
				Help:        "Numero totale di operazioni sui profili (create, modify, delete).",
				ConstLabels: prometheus.Labels{"node_name": nodeName},
			},
			[]string{"operation", "profile_name"},
		),
		managedProfiles: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace:   "kapparmor",
			Name:        "profiles_managed",
			Help:        "Numero totale di profili AppArmor attualmente gestiti.",
			ConstLabels: prometheus.Labels{"node_name": nodeName},
		}),
	}
}

// DefaultProfileMetrics returns the process-wide metrics owner.
func DefaultProfileMetrics() *ProfileMetrics {
	return defaultProfileMetric
}

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

// ProfileCreated increments the create counter.
func ProfileCreated(p string) {
	DefaultProfileMetrics().ProfileCreated(p)
}

// ProfileDeleted increments the delete counter.
func ProfileDeleted(p string) {
	DefaultProfileMetrics().ProfileDeleted(p)
}

// ProfileModified increments the modify counter.
func ProfileModified(p string) {
	DefaultProfileMetrics().ProfileModified(p)
}

// ProfileUpdated kept for compatibility.
func ProfileUpdated(p string) {
	ProfileModified(p)
}

// SetProfileCount sets the gauge to c.
func SetProfileCount(c int) {
	DefaultProfileMetrics().SetManagedProfiles(c)
}

// ProfileCreated increments the create counter.
func (m *ProfileMetrics) ProfileCreated(p string) {
	m.profileOperations.WithLabelValues("create", p).Inc()
}

// ProfileDeleted increments the delete counter.
func (m *ProfileMetrics) ProfileDeleted(p string) {
	m.profileOperations.WithLabelValues("delete", p).Inc()
}

// ProfileModified increments the modify counter.
func (m *ProfileMetrics) ProfileModified(p string) {
	m.profileOperations.WithLabelValues("modify", p).Inc()
}

// SetManagedProfiles sets the managed-profile gauge.
func (m *ProfileMetrics) SetManagedProfiles(c int) {
	m.managedProfiles.Set(float64(c))
}
