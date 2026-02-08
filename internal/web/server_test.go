package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerSetup(t *testing.T) {
	tempDB := filepath.Join(t.TempDir(), "test_server.sqlite3")
	t.Cleanup(func() { os.Remove(tempDB) })

	server, err := New(tempDB, "test-hostname")
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	if server.DB == nil {
		t.Error("expected DB to be initialized")
	}
	if server.Hostname != "test-hostname" {
		t.Errorf("expected hostname 'test-hostname', got %q", server.Hostname)
	}
}

func TestDashboardRequiresAuth(t *testing.T) {
	tempDB := filepath.Join(t.TempDir(), "test_server.sqlite3")
	t.Cleanup(func() { os.Remove(tempDB) })

	server, err := New(tempDB, "test-hostname")
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Test without auth - should redirect
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	server.handleDashboard(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected redirect status 302, got %d", w.Code)
	}
	location := w.Header().Get("Location")
	if !strings.Contains(location, "/__exe.dev/login") {
		t.Errorf("expected redirect to login, got %q", location)
	}
}

func TestDashboardWithAuth(t *testing.T) {
	tempDB := filepath.Join(t.TempDir(), "test_server.sqlite3")
	t.Cleanup(func() { os.Remove(tempDB) })

	server, err := New(tempDB, "test-hostname")
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Test with auth headers
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-ExeDev-UserID", "test-user-123")
	req.Header.Set("X-ExeDev-Email", "test@example.com")
	w := httptest.NewRecorder()
	server.handleDashboard(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Dashboard") {
		t.Error("expected page to contain 'Dashboard'")
	}
	if !strings.Contains(body, "test@example.com") {
		t.Error("expected page to show user email")
	}
}

func TestJobDetailInvalidID(t *testing.T) {
	tempDB := filepath.Join(t.TempDir(), "test_server.sqlite3")
	t.Cleanup(func() { os.Remove(tempDB) })

	server, err := New(tempDB, "test-hostname")
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Test with invalid ID
	req := httptest.NewRequest(http.MethodGet, "/jobs/invalid", nil)
	req.Header.Set("X-ExeDev-UserID", "test-user-123")
	req.Header.Set("X-ExeDev-Email", "test@example.com")
	req.SetPathValue("id", "invalid")
	w := httptest.NewRecorder()
	server.handleJobDetail(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid ID, got %d", w.Code)
	}
}

func TestJobDetailNotFound(t *testing.T) {
	tempDB := filepath.Join(t.TempDir(), "test_server.sqlite3")
	t.Cleanup(func() { os.Remove(tempDB) })

	server, err := New(tempDB, "test-hostname")
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Test with non-existent ID
	req := httptest.NewRequest(http.MethodGet, "/jobs/99999", nil)
	req.Header.Set("X-ExeDev-UserID", "test-user-123")
	req.Header.Set("X-ExeDev-Email", "test@example.com")
	req.SetPathValue("id", "99999")
	w := httptest.NewRecorder()
	server.handleJobDetail(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for non-existent job, got %d", w.Code)
	}
}
