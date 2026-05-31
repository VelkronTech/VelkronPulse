package services

import (
	"strings"
	"testing"
)

func TestValidateEndpointInputRejectsMetadata(t *testing.T) {
	targets := []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://169.254.170.2/",
		"169.254.169.254:80",
	}
	for _, target := range targets {
		endpointType := "http"
		if !strings.HasPrefix(target, "http") {
			endpointType = "tcp"
		}
		if err := ValidateEndpointInput("test", endpointType, target); err == nil {
			t.Fatalf("expected metadata target %q to be rejected", target)
		}
	}
}

func TestValidateEndpointInputRejectsInvalidSchemes(t *testing.T) {
	if err := ValidateEndpointInput("test", "http", "file:///etc/passwd"); err == nil {
		t.Fatal("expected file scheme to be rejected")
	}
}

func TestValidateEndpointInputAllowsLocalhost(t *testing.T) {
	if err := ValidateEndpointInput("local app", "http", "http://127.0.0.1:8080"); err != nil {
		t.Fatalf("expected localhost monitoring target to be allowed, got %v", err)
	}
}

func TestValidateEndpointInputRejectsEmptyName(t *testing.T) {
	if err := ValidateEndpointInput(" ", "http", "http://127.0.0.1:8080"); err == nil {
		t.Fatal("expected empty name to be rejected")
	}
}

func TestValidateEndpointInputRejectsLongName(t *testing.T) {
	name := strings.Repeat("a", MaxEndpointNameLen+1)
	if err := ValidateEndpointInput(name, "http", "http://127.0.0.1:8080"); err == nil {
		t.Fatal("expected long name to be rejected")
	}
}
