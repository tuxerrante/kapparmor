package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// resetMetrics rebuilds the global metrics against a fresh default registry.
func resetMetrics() {
	// Create a fresh registry so we don't conflict with any previously registered metrics.
	reg := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = reg
	prometheus.DefaultGatherer = reg

	// Re-read the node name from the current environment.
	nodeName = getNodeNameFromEnv()

	defaultProfileMetric = newProfileMetrics()
	profileOperations, currentProfiles = aliasesFor(defaultProfileMetric)
}

func TestProfileOperations(t *testing.T) {
	resetMetrics()
	testNodeName := getNodeNameFromEnv()

	// Create operation
	ProfileCreated("profile-a")
	expectedCreate := `
		# HELP kapparmor_profile_operations_total Numero totale di operazioni sui profili (create, modify, delete).
		# TYPE kapparmor_profile_operations_total counter
		kapparmor_profile_operations_total{node_name="` + testNodeName + `",operation="create",profile_name="profile-a"} 1
	`
	if err := testutil.CollectAndCompare(profileOperations, strings.NewReader(expectedCreate), "kapparmor_profile_operations_total"); err != nil {
		t.Errorf("ProfileCreated metric mismatch: %v", err)
	}

	// Modify operation (twice)
	ProfileModified("profile-b")
	ProfileModified("profile-b")
	expectedModify := `
		# HELP kapparmor_profile_operations_total Numero totale di operazioni sui profili (create, modify, delete).
		# TYPE kapparmor_profile_operations_total counter
		kapparmor_profile_operations_total{node_name="` + testNodeName + `",operation="create",profile_name="profile-a"} 1
		kapparmor_profile_operations_total{node_name="` + testNodeName + `",operation="modify",profile_name="profile-b"} 2
	`
	if err := testutil.CollectAndCompare(profileOperations, strings.NewReader(expectedModify), "kapparmor_profile_operations_total"); err != nil {
		t.Errorf("ProfileModified metric mismatch: %v", err)
	}

	// Delete operation
	ProfileDeleted("profile-c")
	expectedDelete := `
		# HELP kapparmor_profile_operations_total Numero totale di operazioni sui profili (create, modify, delete).
		# TYPE kapparmor_profile_operations_total counter
		kapparmor_profile_operations_total{node_name="` + testNodeName + `",operation="create",profile_name="profile-a"} 1
		kapparmor_profile_operations_total{node_name="` + testNodeName + `",operation="modify",profile_name="profile-b"} 2
		kapparmor_profile_operations_total{node_name="` + testNodeName + `",operation="delete",profile_name="profile-c"} 1
	`
	if err := testutil.CollectAndCompare(profileOperations, strings.NewReader(expectedDelete), "kapparmor_profile_operations_total"); err != nil {
		t.Errorf("ProfileDeleted metric mismatch: %v", err)
	}
}

func TestSetProfileCount(t *testing.T) {
	resetMetrics()
	testNodeName := getNodeNameFromEnv()

	SetProfileCount(42)
	expected := `
		# HELP kapparmor_profiles_managed Numero totale di profili AppArmor attualmente gestiti.
		# TYPE kapparmor_profiles_managed gauge
		kapparmor_profiles_managed{node_name="` + testNodeName + `"} 42
	`
	if err := testutil.CollectAndCompare(currentProfiles, strings.NewReader(expected), "kapparmor_profiles_managed"); err != nil {
		t.Errorf("SetProfileCount metric mismatch for 42: %v", err)
	}

	// Verify that the gauge can be updated.
	SetProfileCount(10)
	expectedUpdate := `
		# HELP kapparmor_profiles_managed Numero totale di profili AppArmor attualmente gestiti.
		# TYPE kapparmor_profiles_managed gauge
		kapparmor_profiles_managed{node_name="` + testNodeName + `"} 10
	`
	if err := testutil.CollectAndCompare(currentProfiles, strings.NewReader(expectedUpdate), "kapparmor_profiles_managed"); err != nil {
		t.Errorf("SetProfileCount metric mismatch for 10: %v", err)
	}
}

func TestProfileOperationsDoNotChangeManagedGauge(t *testing.T) {
	resetMetrics()
	testNodeName := getNodeNameFromEnv()

	SetProfileCount(7)
	ProfileCreated("profilo-a")
	ProfileModified("profilo-b")
	ProfileDeleted("profilo-c")

	expected := `
		# HELP kapparmor_profiles_managed Numero totale di profili AppArmor attualmente gestiti.
		# TYPE kapparmor_profiles_managed gauge
		kapparmor_profiles_managed{node_name="` + testNodeName + `"} 7
	`
	if err := testutil.CollectAndCompare(currentProfiles, strings.NewReader(expected), "kapparmor_profiles_managed"); err != nil {
		t.Errorf("Metrica kapparmor_profiles_managed non corrispondente: %v", err)
	}
}

func TestGetNodeNameFromEnv(t *testing.T) {
	// 1. Test with NODE_NAME explicitly set.
	t.Setenv("NODE_NAME", "test-node-123")
	name := getNodeNameFromEnv()
	if name != "test-node-123" {
		t.Errorf("expected 'test-node-123', got %q", name)
	}

	// 2. Without NODE_NAME the hostname should be used.
	t.Setenv("NODE_NAME", "")

	t.Run("HostnameFallback", func(t *testing.T) {
		t.Setenv("NODE_NAME", "")
		hostname, err := os.Hostname()
		if err != nil {
			t.Skipf("unable to read os.Hostname(): %v", err)
		}

		name = getNodeNameFromEnv()
		if name != hostname {
			t.Errorf("expected hostname %q, got %q", hostname, name)
		}
	})
}

func TestMetricsServerHandler(t *testing.T) {
	// Do not test StartMetricsServer directly because it blocks and calls log.Fatalf.
	// Test the HTTP handler instead, which contains the relevant behavior.
	resetMetrics()
	ProfileCreated("test-server-profile")

	// Create a request for /metrics.
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()

	// Get the handler StartMetricsServer would use.
	handler := promhttp.Handler()
	handler.ServeHTTP(rr, req)

	// Check the status code.
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("unexpected status code: got %v, want %v",
			status, http.StatusOK)
	}

	// Check the response body.
	body, _ := io.ReadAll(rr.Body)
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "kapparmor_profile_operations_total") {
		t.Error("response body does not contain kapparmor_profile_operations_total")
	}
	if !strings.Contains(bodyStr, `operation="create",profile_name="test-server-profile"} 1`) {
		t.Error("response body does not contain the expected test-server-profile counter")
	}
}
