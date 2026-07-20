package ingest

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGuardLocalOnly_RejectsCrossOrigin(t *testing.T) {
	h := guardLocalOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	// Present Origin → 403 even from a loopback Host.
	req := httptest.NewRequest("POST", "http://localhost:7475/api/ingest/events", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("cross-origin: want 403, got %d", rec.Code)
	}
	// Rebinding host → 403.
	req2 := httptest.NewRequest("POST", "http://attacker.example/api/ingest/events", nil)
	req2.Host = "attacker.example"
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusForbidden {
		t.Errorf("rebinding host: want 403, got %d", rec2.Code)
	}
	// Legit loopback, no Origin → passes.
	req3 := httptest.NewRequest("POST", "http://127.0.0.1:7475/api/ingest/events", nil)
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Errorf("loopback native client: want 200, got %d", rec3.Code)
	}
}
