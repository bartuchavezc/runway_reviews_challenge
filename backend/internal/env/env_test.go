package env

import (
	"testing"
)

func TestGetString_ReturnsEnvWhenSet(t *testing.T) {
	t.Setenv("TEST_KEY", "hello")
	if got := GetString("TEST_KEY", "fallback"); got != "hello" {
		t.Errorf("expected %q, got %q", "hello", got)
	}
}

func TestGetString_ReturnsFallbackWhenUnset(t *testing.T) {
	if got := GetString("UNSET_KEY_XYZ", "default"); got != "default" {
		t.Errorf("expected %q, got %q", "default", got)
	}
}

func TestGetInt_ReturnsEnvWhenSet(t *testing.T) {
	t.Setenv("TEST_INT", "42")
	if got := GetInt("TEST_INT", 0); got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
}

func TestGetInt_ReturnsFallbackWhenUnset(t *testing.T) {
	if got := GetInt("UNSET_INT_XYZ", 7); got != 7 {
		t.Errorf("expected 7, got %d", got)
	}
}

// GetInt calls log.Fatalf on an invalid value, so the error case is not testable
// in a unit test without subprocess gymnastics. The behavior is intentional:
// a bad env var (e.g. POLL_INTERVAL_MINUTES=abc) must never silently fall back —
// it should prevent the server from starting so the misconfiguration is surfaced.
