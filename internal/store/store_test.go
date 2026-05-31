package store

import (
	"os"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	f, err := os.CreateTemp("", "velkron-pulse-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	path := f.Name()
	f.Close()

	s, err := New(path)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}

	t.Cleanup(func() {
		s.Close()
		os.Remove(path)
	})

	return s
}

func TestNewStore(t *testing.T) {
	s := newTestStore(t)
	if s.db == nil {
		t.Error("expected non-nil db")
	}
}

func TestAddAndListEndpoints(t *testing.T) {
	s := newTestStore(t)

	id, err := s.AddEndpoint("My API", "http://localhost:8080", "http")
	if err != nil {
		t.Fatalf("AddEndpoint failed: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}

	endpoints, err := s.ListEndpoints()
	if err != nil {
		t.Fatalf("ListEndpoints failed: %v", err)
	}
	if len(endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(endpoints))
	}
	if endpoints[0].Name != "My API" {
		t.Errorf("expected name 'My API', got '%s'", endpoints[0].Name)
	}
	if endpoints[0].URL != "http://localhost:8080" {
		t.Errorf("expected URL 'http://localhost:8080', got '%s'", endpoints[0].URL)
	}
	if endpoints[0].Type != "http" {
		t.Errorf("expected type 'http', got '%s'", endpoints[0].Type)
	}
}

func TestDeleteEndpoint(t *testing.T) {
	s := newTestStore(t)

	id, err := s.AddEndpoint("test", "localhost:1234", "tcp")
	if err != nil {
		t.Fatalf("AddEndpoint failed: %v", err)
	}

	if err := s.DeleteEndpoint(id); err != nil {
		t.Fatalf("DeleteEndpoint failed: %v", err)
	}

	endpoints, err := s.ListEndpoints()
	if err != nil {
		t.Fatalf("ListEndpoints failed: %v", err)
	}
	if len(endpoints) != 0 {
		t.Errorf("expected 0 endpoints after delete, got %d", len(endpoints))
	}
}

func TestSettings(t *testing.T) {
	s := newTestStore(t)

	err := s.SetSetting("disk_threshold", "95")
	if err != nil {
		t.Fatalf("SetSetting failed: %v", err)
	}

	val, err := s.GetSetting("disk_threshold")
	if err != nil {
		t.Fatalf("GetSetting failed: %v", err)
	}
	if val != "95" {
		t.Errorf("expected '95', got '%s'", val)
	}

	// Test missing key
	val, err = s.GetSetting("nonexistent")
	if err != nil {
		t.Fatalf("GetSetting failed: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string for missing key, got '%s'", val)
	}

	// Test upsert
	err = s.SetSetting("disk_threshold", "80")
	if err != nil {
		t.Fatalf("SetSetting upsert failed: %v", err)
	}
	val, _ = s.GetSetting("disk_threshold")
	if val != "80" {
		t.Errorf("expected '80' after upsert, got '%s'", val)
	}

	// Get all settings
	all, err := s.GetAllSettings()
	if err != nil {
		t.Fatalf("GetAllSettings failed: %v", err)
	}
	if all["disk_threshold"] != "80" {
		t.Errorf("expected '80', got '%s'", all["disk_threshold"])
	}
}

func TestSaveAndGetMetrics(t *testing.T) {
	s := newTestStore(t)

	err := s.SaveMetrics(45.5, 16000000, 8000000, 50.0, `[{"mount_point":"/","total":100000}]`, `[{"name":"eth0","bytes_sent":500}]`)
	if err != nil {
		t.Fatalf("SaveMetrics failed: %v", err)
	}

	// Verify the row was inserted by querying the whole table
	var count int
	err = s.db.QueryRow("SELECT COUNT(*) FROM metrics_snapshots WHERE cpu_percent = 45.5").Scan(&count)
	if err != nil {
		t.Fatalf("Count query failed: %v", err)
	}
	if count < 1 {
		t.Error("expected at least 1 metrics snapshot with CPU 45.5")
	}
}

func TestMarshalToJSON(t *testing.T) {
	type test struct {
		A string `json:"a"`
		B int    `json:"b"`
	}
	input := test{A: "hello", B: 42}
	output, err := MarshalToJSON(input)
	if err != nil {
		t.Fatalf("MarshalToJSON failed: %v", err)
	}
	expected := `{"a":"hello","b":42}`
	if output != expected {
		t.Errorf("expected %s, got %s", expected, output)
	}

	// Test empty slice
	empty, err := MarshalToJSON([]int{})
	if err != nil {
		t.Fatalf("MarshalToJSON empty failed: %v", err)
	}
	if empty != "[]" {
		t.Errorf("expected [], got %s", empty)
	}
}
