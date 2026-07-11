package avatar

import (
	"context"
	"testing"
	"time"
)

func TestSetup_NoProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
	}{
		{"empty string", ""},
		{"none", "none"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Setup(SetupConfig{Provider: tt.provider})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Mode != SetupModeNone {
				t.Errorf("expected mode %q, got %q", SetupModeNone, result.Mode)
			}
			if result.Session != nil {
				t.Error("expected nil session for none mode")
			}
			if result.StaticImage != nil {
				t.Error("expected nil StaticImage for none mode")
			}
		})
	}
}

func TestSetup_StaticProvider(t *testing.T) {
	tests := []struct {
		name     string
		cfg      SetupConfig
		wantPath string
		wantData bool
	}{
		{
			name: "with H264Path",
			cfg: SetupConfig{
				Provider: ProviderStatic,
				StaticImage: StaticImageConfig{
					H264Path: "/path/to/avatar.h264",
				},
			},
			wantPath: "/path/to/avatar.h264",
		},
		{
			name: "with H264Data",
			cfg: SetupConfig{
				Provider: ProviderStatic,
				StaticImage: StaticImageConfig{
					H264Data: []byte{0x00, 0x00, 0x01},
				},
			},
			wantData: true,
		},
		{
			name: "with UseDefault",
			cfg: SetupConfig{
				Provider: ProviderStatic,
				StaticImage: StaticImageConfig{
					UseDefault: true,
					H264Path:   "/should/be/ignored",
				},
			},
			wantPath: "", // UseDefault ignores H264Path
		},
		{
			name: "empty static config",
			cfg: SetupConfig{
				Provider: ProviderStatic,
			},
			wantPath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Setup(tt.cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Mode != SetupModeStatic {
				t.Errorf("expected mode %q, got %q", SetupModeStatic, result.Mode)
			}
			if result.Session != nil {
				t.Error("expected nil session for static mode")
			}
			if result.StaticImage == nil {
				t.Fatal("expected non-nil StaticImage for static mode")
			}
			if result.StaticImage.H264Path != tt.wantPath {
				t.Errorf("expected H264Path %q, got %q", tt.wantPath, result.StaticImage.H264Path)
			}
			if tt.wantData && len(result.StaticImage.H264Data) == 0 {
				t.Error("expected non-empty H264Data")
			}
		})
	}
}

func TestSetup_UnregisteredProvider(t *testing.T) {
	_, err := Setup(SetupConfig{Provider: "nonexistent"})
	if err == nil {
		t.Error("expected error for unregistered provider")
	}
}

func TestSetup_LiveProvider_Registered(t *testing.T) {
	// Register a mock provider for testing
	RegisterProvider("test-live", func(cfg Config) (Session, error) {
		return newMockSession("test-live"), nil
	})

	result, err := Setup(SetupConfig{Provider: "test-live"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Mode != SetupModeLive {
		t.Errorf("expected mode %q, got %q", SetupModeLive, result.Mode)
	}
	if result.Session == nil {
		t.Fatal("expected non-nil session for live mode")
	}
	if result.Session.Provider() != "test-live" {
		t.Errorf("expected provider %q, got %q", "test-live", result.Session.Provider())
	}
}

// mockSession implements Session for testing
type mockSession struct {
	*BaseSession
	providerName string
}

func newMockSession(provider string) *mockSession {
	return &mockSession{
		BaseSession:  NewBaseSession(provider, "mock-avatar"),
		providerName: provider,
	}
}

func (m *mockSession) Provider() string {
	if m.providerName != "" {
		return m.providerName
	}
	return "mock"
}

func (m *mockSession) Start(_ context.Context, _ StartOptions) error {
	return nil
}

func (m *mockSession) WaitForJoin(_ context.Context, _ time.Duration) error {
	return nil
}

func (m *mockSession) Close(_ context.Context) error {
	return nil
}

func TestSetupMode_String(t *testing.T) {
	modes := []SetupMode{SetupModeNone, SetupModeStatic, SetupModeLive}
	expected := []string{"none", "static", "live"}

	for i, mode := range modes {
		if string(mode) != expected[i] {
			t.Errorf("expected %q, got %q", expected[i], string(mode))
		}
	}
}
