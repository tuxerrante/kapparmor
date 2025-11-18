package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// resetMetrics è una funzione di utilità per resettare le metriche globali tra i test.
// Questo è necessario perché promauto registra metriche globali.
func resetMetrics() {
	// Create a fresh registry so we don't conflict with any previously registered metrics.
	reg := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = reg
	prometheus.DefaultGatherer = reg

	// Assicura che nodeName sia resettato in base all'ambiente
	nodeName = getNodeNameFromEnv()

	// Ricrea le metriche (promauto le registra automaticamente sul DefaultRegisterer, ora resettato)
	profileOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   "kapparmor",
			Name:        "profile_operations_total",
			Help:        "Numero totale di operazioni sui profili (create, modify, delete).",
			ConstLabels: prometheus.Labels{"node_name": nodeName},
		},
		[]string{"operation", "profile_name"},
	)

	currentProfiles = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace:   "kapparmor",
			Name:        "profiles_managed",
			Help:        "Numero totale di profili AppArmor attualmente gestiti.",
			ConstLabels: prometheus.Labels{"node_name": nodeName},
		},
	)
}

func TestProfileOperations(t *testing.T) {
	resetMetrics()
	testNodeName := getNodeNameFromEnv() // Ottieni il nome del nodo per il confronto

	// Test Creazione
	ProfileCreated("profilo-a")
	expectedCreate := `
		# HELP kapparmor_profile_operations_total Numero totale di operazioni sui profili (create, modify, delete).
		# TYPE kapparmor_profile_operations_total counter
		kapparmor_profile_operations_total{node_name="` + testNodeName + `",operation="create",profile_name="profilo-a"} 1
	`
	if err := testutil.CollectAndCompare(profileOperations, strings.NewReader(expectedCreate), "kapparmor_profile_operations_total"); err != nil {
		t.Errorf("Metrica ProfileCreated non corrispondente: %v", err)
	}

	// Test Modifica (due volte)
	ProfileModified("profilo-b")
	ProfileModified("profilo-b")
	expectedModify := `
		# HELP kapparmor_profile_operations_total Numero totale di operazioni sui profili (create, modify, delete).
		# TYPE kapparmor_profile_operations_total counter
		kapparmor_profile_operations_total{node_name="` + testNodeName + `",operation="create",profile_name="profilo-a"} 1
		kapparmor_profile_operations_total{node_name="` + testNodeName + `",operation="modify",profile_name="profilo-b"} 2
	`
	if err := testutil.CollectAndCompare(profileOperations, strings.NewReader(expectedModify), "kapparmor_profile_operations_total"); err != nil {
		t.Errorf("Metrica ProfileModified non corrispondente: %v", err)
	}

	// Test Eliminazione
	ProfileDeleted("profilo-c")
	expectedDelete := `
		# HELP kapparmor_profile_operations_total Numero totale di operazioni sui profili (create, modify, delete).
		# TYPE kapparmor_profile_operations_total counter
		kapparmor_profile_operations_total{node_name="` + testNodeName + `",operation="create",profile_name="profilo-a"} 1
		kapparmor_profile_operations_total{node_name="` + testNodeName + `",operation="modify",profile_name="profilo-b"} 2
		kapparmor_profile_operations_total{node_name="` + testNodeName + `",operation="delete",profile_name="profilo-c"} 1
	`
	if err := testutil.CollectAndCompare(profileOperations, strings.NewReader(expectedDelete), "kapparmor_profile_operations_total"); err != nil {
		t.Errorf("Metrica ProfileDeleted non corrispondente: %v", err)
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
		t.Errorf("Metrica SetProfileCount (42) non corrispondente: %v", err)
	}

	// Testa l'aggiornamento del gauge
	SetProfileCount(10)
	expectedUpdate := `
		# HELP kapparmor_profiles_managed Numero totale di profili AppArmor attualmente gestiti.
		# TYPE kapparmor_profiles_managed gauge
		kapparmor_profiles_managed{node_name="` + testNodeName + `"} 10
	`
	if err := testutil.CollectAndCompare(currentProfiles, strings.NewReader(expectedUpdate), "kapparmor_profiles_managed"); err != nil {
		t.Errorf("Metrica SetProfileCount (10) non corrispondente: %v", err)
	}
}

func TestGetNodeNameFromEnv(t *testing.T) {
	// 1. Test con NODE_NAME impostato
	t.Setenv("NODE_NAME", "test-nodo-123")
	name := getNodeNameFromEnv()
	if name != "test-nodo-123" {
		t.Errorf("Previsto 'test-nodo-123', ottenuto '%s'", name)
	}

	// 2. Test senza NODE_NAME (dovrebbe usare os.Hostname)
	// Rimuove la variabile d'ambiente impostata da t.Setenv
	t.Setenv("NODE_NAME", "") // Simula l'unset per questo sub-test

	// Ricrea un test "pulito" per l'unset
	t.Run("HostnameFallback", func(t *testing.T) {
		t.Setenv("NODE_NAME", "") // Assicura che sia vuota
		hostname, err := os.Hostname()
		if err != nil {
			t.Skipf("Impossible ottenere os.Hostname(): %v", err)
		}

		name = getNodeNameFromEnv()
		if name != hostname {
			t.Errorf("Previsto hostname '%s', ottenuto '%s'", hostname, name)
		}
	})
}

func TestMetricsServerHandler(t *testing.T) {
	// Non testiamo StartMetricsServer direttamente perché è bloccante e chiama log.Fatalf.
	// Testiamo invece l'handler che registra, che è la parte logica cruciale.
	resetMetrics()
	ProfileCreated("test-server-profilo")

	// Crea una richiesta di test per /metrics
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()

	// Ottieni l'handler che StartMetricsServer userebbe
	handler := promhttp.Handler()
	handler.ServeHTTP(rr, req)

	// Controlla lo status code
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("L'handler ha restituito uno status code errato: ottenuto %v, previsto %v",
			status, http.StatusOK)
	}

	// Controlla il body
	body, _ := io.ReadAll(rr.Body)
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "kapparmor_profile_operations_total") {
		t.Error("Il body della risposta non contiene la metrica 'kapparmor_profile_operations_total'")
	}
	if !strings.Contains(bodyStr, `operation="create",profile_name="test-server-profilo"} 1`) {
		t.Error("Il body della risposta non contiene il valore corretto per 'test-server-profilo'")
	}
}
