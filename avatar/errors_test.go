package avatar

import (
	"errors"
	"testing"
)

func TestProviderErrorError(t *testing.T) {
	tests := []struct {
		name     string
		err      *ProviderError
		expected string
	}{
		{
			name: "with underlying error",
			err: &ProviderError{
				Provider: "tavus",
				Op:       "create_session",
				Err:      errors.New("connection refused"),
			},
			expected: "avatar/tavus: create_session: connection refused",
		},
		{
			name: "without underlying error",
			err: &ProviderError{
				Provider: "anam",
				Op:       "send_audio",
			},
			expected: "avatar/anam: send_audio",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestProviderErrorUnwrap(t *testing.T) {
	underlying := errors.New("underlying error")
	err := &ProviderError{
		Provider: "simli",
		Op:       "connect",
		Err:      underlying,
	}

	unwrapped := err.Unwrap()
	if unwrapped != underlying {
		t.Errorf("expected %v, got %v", underlying, unwrapped)
	}

	// Test errors.Is works through wrapping
	if !errors.Is(err, underlying) {
		t.Error("errors.Is should return true for underlying error")
	}
}

func TestNewProviderError(t *testing.T) {
	underlying := errors.New("test error")
	err := NewProviderError("tavus", "init", underlying)

	if err.Provider != "tavus" {
		t.Errorf("expected Provider 'tavus', got %s", err.Provider)
	}
	if err.Op != "init" {
		t.Errorf("expected Op 'init', got %s", err.Op)
	}
	if err.Err != underlying {
		t.Errorf("expected Err %v, got %v", underlying, err.Err)
	}
}

func TestSentinelErrors(t *testing.T) {
	// Test that all sentinel errors are distinct
	errs := []error{
		ErrSessionNotStarted,
		ErrSessionAlreadyStarted,
		ErrAvatarJoinTimeout,
		ErrAvatarTrackTimeout,
		ErrProviderUnavailable,
		ErrProviderAuthFailed,
		ErrProviderRateLimited,
		ErrInvalidConfig,
		ErrRPCTimeout,
		ErrRPCFailed,
		ErrStreamClosed,
		ErrAvatarDisconnected,
	}

	for i, err1 := range errs {
		for j, err2 := range errs {
			if i != j && errors.Is(err1, err2) {
				t.Errorf("sentinel errors should be distinct: %v == %v", err1, err2)
			}
		}
	}

	// Test that sentinel errors have meaningful messages
	for _, err := range errs {
		msg := err.Error()
		if msg == "" {
			t.Errorf("sentinel error should have non-empty message: %v", err)
		}
		if len(msg) < 10 {
			t.Errorf("sentinel error message seems too short: %s", msg)
		}
	}
}

func TestProviderErrorWrapping(t *testing.T) {
	// Test wrapping a sentinel error
	wrapped := NewProviderError("tavus", "connect", ErrProviderUnavailable)

	if !errors.Is(wrapped, ErrProviderUnavailable) {
		t.Error("wrapped error should match underlying sentinel")
	}

	// Test double wrapping
	doubleWrapped := NewProviderError("tavus", "retry", wrapped)

	if !errors.Is(doubleWrapped, ErrProviderUnavailable) {
		t.Error("double wrapped error should still match underlying sentinel")
	}
}
