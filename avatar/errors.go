package avatar

import "errors"

// Sentinel errors for avatar operations.
var (
	// ErrSessionNotStarted indicates that Start() was not called before
	// attempting an operation that requires an active session.
	ErrSessionNotStarted = errors.New("avatar: session not started")

	// ErrSessionAlreadyStarted indicates that Start() was called on a
	// session that is already running.
	ErrSessionAlreadyStarted = errors.New("avatar: session already started")

	// ErrAvatarJoinTimeout indicates that the avatar participant did not
	// join the room within the specified timeout.
	ErrAvatarJoinTimeout = errors.New("avatar: join timeout")

	// ErrAvatarTrackTimeout indicates that the avatar did not publish
	// the expected track (video/audio) within the timeout.
	ErrAvatarTrackTimeout = errors.New("avatar: track publish timeout")

	// ErrProviderUnavailable indicates that the avatar provider API
	// is unreachable or returned an error.
	ErrProviderUnavailable = errors.New("avatar: provider unavailable")

	// ErrProviderAuthFailed indicates that authentication with the
	// avatar provider failed (invalid API key, etc.).
	ErrProviderAuthFailed = errors.New("avatar: provider authentication failed")

	// ErrProviderRateLimited indicates that the avatar provider has
	// rate-limited the request.
	ErrProviderRateLimited = errors.New("avatar: provider rate limited")

	// ErrInvalidConfig indicates that the avatar configuration is
	// invalid or incomplete.
	ErrInvalidConfig = errors.New("avatar: invalid configuration")

	// ErrRPCTimeout indicates that an RPC call to the avatar timed out.
	ErrRPCTimeout = errors.New("avatar: RPC timeout")

	// ErrRPCFailed indicates that an RPC call to the avatar failed.
	ErrRPCFailed = errors.New("avatar: RPC failed")

	// ErrStreamClosed indicates that the audio stream was closed
	// unexpectedly.
	ErrStreamClosed = errors.New("avatar: stream closed")

	// ErrAvatarDisconnected indicates that the avatar participant
	// disconnected from the room unexpectedly.
	ErrAvatarDisconnected = errors.New("avatar: disconnected")
)

// ProviderError wraps an error from an avatar provider with additional context.
type ProviderError struct {
	Provider string // Provider name (e.g., "tavus", "anam")
	Op       string // Operation that failed
	Err      error  // Underlying error
}

// Error implements the error interface.
func (e *ProviderError) Error() string {
	if e.Err != nil {
		return "avatar/" + e.Provider + ": " + e.Op + ": " + e.Err.Error()
	}
	return "avatar/" + e.Provider + ": " + e.Op
}

// Unwrap returns the underlying error.
func (e *ProviderError) Unwrap() error {
	return e.Err
}

// NewProviderError creates a new ProviderError.
func NewProviderError(provider, op string, err error) *ProviderError {
	return &ProviderError{
		Provider: provider,
		Op:       op,
		Err:      err,
	}
}
