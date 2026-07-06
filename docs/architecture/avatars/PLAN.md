# Lip-Sync Avatar Feature - Implementation Plan

**Feature**: Real-Time Lip-Sync Avatars for Voice Agents
**Status**: Phase 0-2 Complete (v0.2.0), Phases 3-6 Future Work
**Author**: PlexusOne
**Created**: 2025-07-06
**Last Updated**: 2026-07-06

## Implementation Phases

### Phase 0: SDK Gap Analysis (1-2 days)

Before implementation, verify LiveKit Go SDK capabilities.

#### Tasks

- [ ] **P0-1**: Check ByteStream support in `livekit/server-sdk-go`
  - Search for `StreamBytes`, `ByteStreamWriter`, `DataStream`
  - If missing, evaluate implementation effort

- [ ] **P0-2**: Check RPC support in `livekit/server-sdk-go`
  - Search for `PerformRPC`, `RegisterRPCMethod`, `RPC`
  - If missing, evaluate implementation effort

- [ ] **P0-3**: Check `lk.publish_on_behalf` attribute support
  - Verify token generation supports custom attributes
  - Test with a simple avatar-style participant

- [ ] **P0-4**: Document gaps and decide on approach
  - Option A: Contribute upstream to LiveKit
  - Option B: Implement locally in omni-livekit
  - Option C: Use alternative transport (DataChannel, WebSocket)

#### Deliverables

- Gap analysis document
- Decision on implementation approach
- Updated timeline if gaps discovered

---

### Phase 1: Core Avatar Infrastructure (3-5 days)

Build the foundation that all avatar providers will use.

#### 1.1 Avatar Package Structure

```bash
mkdir -p avatar
touch avatar/doc.go
touch avatar/session.go
touch avatar/audio.go
touch avatar/rpc.go
touch avatar/token.go
touch avatar/errors.go
touch avatar/metrics.go
```

#### 1.2 Core Interfaces

**File**: `avatar/session.go`

- [ ] Define `Session` interface
- [ ] Define `StartOptions` struct
- [ ] Define `SessionCallbacks` struct
- [ ] Define `Metrics` struct

**File**: `avatar/audio.go`

- [ ] Define `AudioDestination` interface
- [ ] Define `PlaybackEvent` types
- [ ] Define `PlaybackCallback` type

#### 1.3 Token Generation

**File**: `avatar/token.go`

- [ ] Implement `GenerateAvatarToken()`
- [ ] Support `lk.publish_on_behalf` attribute
- [ ] Support configurable TTL
- [ ] Add unit tests

#### 1.4 Error Types

**File**: `avatar/errors.go`

- [ ] Define sentinel errors
- [ ] Create error wrapping helpers

#### 1.5 DataStream Audio Output

**File**: `avatar/datastream.go`

- [ ] Implement `DataStreamAudioOutput` struct
- [ ] Implement `CaptureFrame()` method
- [ ] Implement `Flush()` method
- [ ] Implement `ClearBuffer()` method
- [ ] Register RPC handlers for playback callbacks
- [ ] Add unit tests with mock room

#### 1.6 Queue Audio Output (Testing)

**File**: `avatar/queue.go`

- [ ] Implement `QueueAudioOutput` for local testing
- [ ] In-memory queue without network
- [ ] Useful for unit tests and debugging

#### Deliverables

- `avatar/` package with core interfaces
- Token generation with tests
- DataStreamAudioOutput with tests (mocked)
- QueueAudioOutput for testing

---

### Phase 2: First Provider - Tavus (3-4 days)

Implement the first avatar provider to validate the architecture.

#### 2.1 Tavus API Client

**File**: `avatar/tavus/client.go`

- [ ] Implement HTTP client for Tavus API
- [ ] `CreateConversation()` method
- [ ] `EndConversation()` method
- [ ] Error handling and retries
- [ ] Add unit tests with httptest

#### 2.2 Tavus Avatar Session

**File**: `avatar/tavus/session.go`

- [ ] Implement `AvatarSession` struct
- [ ] Implement `Session` interface
- [ ] `Start()`: Create conversation, generate token, configure audio
- [ ] `WaitForJoin()`: Wait for avatar participant
- [ ] `Close()`: End conversation, remove participant
- [ ] Add unit tests

#### 2.3 Integration Test

**File**: `avatar/tavus/integration_test.go`

- [ ] End-to-end test with real Tavus API
- [ ] Requires `TAVUS_API_KEY` env var
- [ ] Skip if credentials not available

#### Deliverables

- Working Tavus integration
- Integration test passing
- Documentation for Tavus setup

---

### Phase 3: Second Provider - Anam (2-3 days)

Validate architecture with a second provider.

#### 3.1 Anam API Client

**File**: `avatar/anam/client.go`

- [ ] Implement HTTP client for Anam API
- [ ] `CreateSessionToken()` method
- [ ] `StartEngineSession()` method
- [ ] Error handling

#### 3.2 Anam Avatar Session

**File**: `avatar/anam/session.go`

- [ ] Implement `AvatarSession` struct
- [ ] Implement `Session` interface
- [ ] Provider-specific configuration

#### 3.3 Integration Test

**File**: `avatar/anam/integration_test.go`

- [ ] End-to-end test with real Anam API

#### Deliverables

- Working Anam integration
- Validated that architecture supports multiple providers

---

### Phase 4: Third Provider - Simli (2-3 days)

Add a third provider for broader coverage.

#### 4.1 Simli API Client

**File**: `avatar/simli/client.go`

- [ ] Implement HTTP client for Simli API
- [ ] Token/session management

#### 4.2 Simli Avatar Session

**File**: `avatar/simli/session.go`

- [ ] Implement `AvatarSession` struct
- [ ] Implement `Session` interface

#### 4.3 Integration Test

**File**: `avatar/simli/integration_test.go`

- [ ] End-to-end test with real Simli API

#### Deliverables

- Working Simli integration
- Three providers validated

---

### Phase 5: Agent Integration (2-3 days)

Integrate avatar support into the existing agent workflow.

#### 5.1 Agent Options

**File**: `agent/options.go`

- [ ] Add `AvatarConfig` to Options
- [ ] Add `AvatarProvider` field
- [ ] Add `AvatarAPIKey` field

#### 5.2 Agent Avatar Methods

**File**: `agent/avatar.go`

- [ ] Add `StartAvatar()` method
- [ ] Add `StopAvatar()` method
- [ ] Add `AvatarSession()` getter
- [ ] Handle avatar in Leave()

#### 5.3 Audio Routing

- [ ] Modify audio output to support avatar destination
- [ ] Option to route TTS to avatar instead of direct publish
- [ ] Support interruption flow through avatar

#### 5.4 Examples

**File**: `cmd/avatar-agent/main.go`

- [ ] Example voice agent with avatar
- [ ] Environment variable configuration
- [ ] README with setup instructions

#### Deliverables

- Avatar support in Agent struct
- Example application
- Updated documentation

---

### Phase 6: Documentation & Polish (2-3 days)

Finalize documentation and developer experience.

#### 6.1 User Documentation

- [ ] Update `docs/guides/voice-agent.md` with avatar section
- [ ] Create `docs/guides/lip-sync-avatars.md`
- [ ] Update `docs/architecture/avatar.md` to cover lip-sync

#### 6.2 API Documentation

- [ ] Create `docs/api/avatar.md`
- [ ] Document each provider's configuration
- [ ] Add troubleshooting guide

#### 6.3 MkDocs Integration

- [ ] Add new pages to `mkdocs.yml`
- [ ] Build and verify site

#### 6.4 README Updates

- [ ] Add avatar section to main README
- [ ] Update feature list

#### Deliverables

- Complete documentation
- Updated MkDocs site
- README with avatar info

---

## Timeline Summary

| Phase | Duration | Dependencies |
|-------|----------|--------------|
| Phase 0: SDK Gap Analysis | 1-2 days | None |
| Phase 1: Core Infrastructure | 3-5 days | Phase 0 |
| Phase 2: Tavus Provider | 3-4 days | Phase 1 |
| Phase 3: Anam Provider | 2-3 days | Phase 1 |
| Phase 4: Simli Provider | 2-3 days | Phase 1 |
| Phase 5: Agent Integration | 2-3 days | Phase 2 |
| Phase 6: Documentation | 2-3 days | Phase 5 |

**Total Estimate**: 15-23 days (3-5 weeks)

**Note**: Phases 2-4 (providers) can be parallelized if multiple developers are available.

---

## Task Checklist

### Phase 0: SDK Gap Analysis ✅

- [x] P0-1: Check ByteStream support
- [x] P0-2: Check RPC support
- [x] P0-3: Check publish_on_behalf support
- [x] P0-4: Document gaps and decide approach

### Phase 1: Core Infrastructure ✅

- [x] P1-1: Create package structure
- [x] P1-2: Define Session interface
- [x] P1-3: Define AudioDestination interface
- [x] P1-4: Implement token generation
- [x] P1-5: Define error types
- [x] P1-6: Implement DataStreamAudioOutput
- [x] P1-7: Implement QueueAudioOutput
- [x] P1-8: Write unit tests

### Phase 2: Tavus Provider ✅

- [x] P2-1: Implement Tavus API client (via [tavus-go](https://github.com/plexusone/tavus-go) SDK v0.2.0)
- [x] P2-2: Implement TavusAvatarSession
- [x] P2-3: Write unit tests
- [x] P2-4: Write integration test (skips without TAVUS_API_KEY)
- [x] P2-5: Document Tavus configuration

### Phase 3: Anam Provider (Future Work)

- [ ] P3-1: Implement Anam API client
- [ ] P3-2: Implement AnamAvatarSession
- [ ] P3-3: Write tests
- [ ] P3-4: Document Anam configuration

### Phase 4: Simli Provider (Future Work)

- [ ] P4-1: Implement Simli API client
- [ ] P4-2: Implement SimliAvatarSession
- [ ] P4-3: Write tests
- [ ] P4-4: Document Simli configuration

### Phase 5: Agent Integration (Future Work)

- [ ] P5-1: Add AvatarConfig to agent options
- [ ] P5-2: Implement StartAvatar/StopAvatar
- [ ] P5-3: Integrate audio routing
- [ ] P5-4: Create example application
- [ ] P5-5: Write integration test

### Phase 6: Documentation (Partial - v0.2.0)

- [ ] P6-1: Update voice-agent guide
- [ ] P6-2: Create lip-sync-avatars guide
- [ ] P6-3: Create avatar API reference
- [x] P6-4: Update mkdocs.yml (v0.2.0 release notes added)
- [x] P6-5: Update README (voice pipeline, Tavus avatars section added)

---

## Risk Mitigation

### Risk: LiveKit Go SDK lacks ByteStream/RPC

**Mitigation**: Phase 0 identifies this early. Options:

1. **Contribute upstream** (2-4 weeks): Submit PR to LiveKit
2. **Implement locally** (1-2 weeks): Build on DataChannel
3. **WebSocket fallback** (1 week): Direct WebSocket to avatar

### Risk: Provider API changes

**Mitigation**: Abstract all providers behind `Session` interface. Pin API versions where possible.

### Risk: Latency too high

**Mitigation**: Choose providers with proven low latency (Simli, Anam). Add latency metrics to detect issues early.

### Risk: Integration complexity

**Mitigation**: Start with simplest provider (Anam has "10 minute quickstart"). Validate architecture before adding complexity.

---

## Definition of Done

### Per Provider

- [ ] API client implemented and tested
- [ ] Session interface implemented
- [ ] Unit tests passing
- [ ] Integration test passing (with real API)
- [ ] Configuration documented
- [ ] Error handling complete

### Overall Feature

- [ ] All Phase 1-6 tasks complete
- [ ] At least 2 providers working
- [ ] Example application working
- [ ] Documentation complete
- [ ] MkDocs site updated
- [ ] README updated
- [ ] No critical bugs open

---

## Related Documents

- [PRD.md](PRD.md) - Product Requirements Document
- [TRD.md](TRD.md) - Technical Requirements Document
- [ROADMAP.md](ROADMAP.md) - Feature Roadmap
