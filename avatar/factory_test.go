package avatar

import (
	"errors"
	"testing"
)

func TestNewSession_NoProvider(t *testing.T) {
	// Empty provider should return nil, nil (audio-only mode)
	session, err := NewSession(Config{Provider: ""})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if session != nil {
		t.Errorf("expected nil session for empty provider")
	}

	// ProviderNone constant should also work
	session, err = NewSession(Config{Provider: ProviderNone})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if session != nil {
		t.Errorf("expected nil session for ProviderNone")
	}
}

func TestNewSession_StaticProvider(t *testing.T) {
	// Static provider should return an error (not Session-based)
	session, err := NewSession(Config{Provider: ProviderStatic})
	if err == nil {
		t.Error("expected error for static provider")
	}
	if session != nil {
		t.Error("expected nil session for static provider")
	}
}

func TestNewSession_UnregisteredProvider(t *testing.T) {
	// Unregistered provider should return an error
	session, err := NewSession(Config{Provider: "nonexistent"})
	if err == nil {
		t.Error("expected error for unregistered provider")
	}
	if session != nil {
		t.Error("expected nil session for unregistered provider")
	}
}

func TestRegisterProvider(t *testing.T) {
	// Register a test provider
	called := false
	RegisterProvider("test-provider", func(cfg Config) (Session, error) {
		called = true
		return nil, errors.New("test error")
	})

	// Verify it's registered
	if !IsProviderRegistered("test-provider") {
		t.Error("expected test-provider to be registered")
	}

	// Verify it appears in the list
	providers := RegisteredProviders()
	found := false
	for _, p := range providers {
		if p == "test-provider" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected test-provider in RegisteredProviders()")
	}

	// Verify the constructor is called
	_, _ = NewSession(Config{Provider: "test-provider"})
	if !called {
		t.Error("expected test constructor to be called")
	}
}

func TestIsProviderRegistered(t *testing.T) {
	// Unregistered provider
	if IsProviderRegistered("definitely-not-registered") {
		t.Error("expected false for unregistered provider")
	}

	// Register and check
	RegisterProvider("check-test", func(cfg Config) (Session, error) {
		return nil, nil
	})
	if !IsProviderRegistered("check-test") {
		t.Error("expected true for registered provider")
	}
}
