package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kelsos/rotki-sync/internal/config"
)

// newMockBackend returns a test server that registers every /api/1 route except
// those in the missing set, mimicking rotki's route table for OPTIONS probes.
func newMockBackend(missing ...string) *httptest.Server {
	gone := make(map[string]bool, len(missing))
	for _, m := range missing {
		gone["/api/1"+m] = true
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gone[r.URL.Path] {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
}

func TestPreflightEndpointsAllPresent(t *testing.T) {
	server := newMockBackend()
	defer server.Close()

	svc := NewSyncService(&config.Config{BaseURL: server.URL})
	if err := svc.PreflightEndpoints(); err != nil {
		t.Fatalf("expected preflight to pass, got: %v", err)
	}
}

// TestPreflightCatchesRemovedEndpoint reproduces the incident: a transaction
// endpoint was removed by a core upgrade. The preflight must catch it.
func TestPreflightCatchesRemovedEndpoint(t *testing.T) {
	server := newMockBackend(evmTransactionsEndpoint, transactionsDecodeEndpoint)
	defer server.Close()

	svc := NewSyncService(&config.Config{BaseURL: server.URL})
	err := svc.PreflightEndpoints()
	if err == nil {
		t.Fatal("expected preflight to fail when transaction endpoints are missing")
	}
	if !strings.Contains(err.Error(), evmTransactionsEndpoint) {
		t.Errorf("error should name the missing fetch endpoint: %v", err)
	}
	if !strings.Contains(err.Error(), transactionsDecodeEndpoint) {
		t.Errorf("error should name the missing decode endpoint: %v", err)
	}
}
