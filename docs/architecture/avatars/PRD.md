# Lip-Sync Avatar Feature - Product Requirements Document

**Feature**: Real-Time Lip-Sync Avatars for Voice Agents
**Status**: Draft
**Author**: PlexusOne
**Created**: 2025-07-06
**Last Updated**: 2025-07-06

## Executive Summary

Enable omni-livekit voice agents to display real-time lip-synced video avatars that move their mouths in sync with generated speech. This transforms voice-only agents into visually engaging AI participants that feel more human and present in video meetings.

## Problem Statement

### Current State

Voice agents in omni-livekit can:

- Join LiveKit rooms and participate in meetings
- Listen to participant audio (Opus decode)
- Speak via TTS (PCM encode to Opus)
- Display a static image avatar (H.264 keyframe)

### Gap

Static avatars lack visual engagement. Users report that:

- Voice-only agents feel "robotic" and disconnected
- Static images don't convey that the agent is speaking
- The experience is inferior to human participants with webcams

### Opportunity

Lip-sync avatar technology has matured significantly:

- Multiple providers offer real-time avatar APIs (Tavus, Anam, Simli, D-ID, HeyGen)
- Latency is now acceptable (~100-300ms)
- LiveKit's Python/JS SDKs already integrate these providers
- **Go has no equivalent solution** - we can be first-to-market

## Goals

### Primary Goals

1. **Enable lip-sync avatars** for omni-livekit voice agents
2. **Support multiple providers** via a pluggable architecture
3. **Maintain Go-native implementation** without Python/JS dependencies
4. **Achieve <500ms lip-sync latency** for natural conversation

### Secondary Goals

1. Port LiveKit's Python/JS avatar architecture to Go
2. Create reusable avatar SDK for broader Go ecosystem
3. Enable hybrid architectures (Go agent + external avatar worker)

### Non-Goals

1. Building our own lip-sync/video generation (use existing providers)
2. Local avatar generation on the agent host (too computationally expensive)
3. Supporting non-LiveKit video platforms

## User Stories

### Voice Agent Developer

> As a developer building voice agents in Go, I want to add a lip-sync avatar to my agent so that users have a more engaging experience without switching to Python.

**Acceptance Criteria**:

- Can add avatar support with <20 lines of code
- Avatar provider is configurable (Tavus, Anam, etc.)
- Works with existing omni-livekit agent code
- No Python/JS runtime required

### Meeting Participant

> As a meeting participant, I want the AI agent to show a face that moves when it speaks so that I know when the agent is talking and feel more connected to it.

**Acceptance Criteria**:

- Avatar appears in video tile alongside human participants
- Mouth movements are synchronized with speech
- Avatar responds naturally to interruptions (stops speaking)
- Visual quality is comparable to a webcam feed

### Enterprise IT Administrator

> As an IT administrator, I want to configure which avatar provider we use so that I can control costs and ensure compliance with our data policies.

**Acceptance Criteria**:

- Avatar provider configurable via environment variables
- API keys securely managed
- Can disable avatars entirely if needed
- Audit logging of avatar API calls

## Functional Requirements

### FR-1: Avatar Session Lifecycle

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-1.1 | Agent can start an avatar session when joining a room | P0 |
| FR-1.2 | Avatar joins the room as a separate participant | P0 |
| FR-1.3 | Avatar publishes video+audio tracks on behalf of the agent | P0 |
| FR-1.4 | Avatar session can be closed when agent leaves | P0 |
| FR-1.5 | Avatar reconnects automatically on transient failures | P1 |

### FR-2: Audio Streaming

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-2.1 | Agent streams TTS audio to avatar via LiveKit ByteStream | P0 |
| FR-2.2 | Audio format: PCM16, 16kHz or 24kHz (provider-dependent) | P0 |
| FR-2.3 | Audio segments are demarcated for playback tracking | P0 |
| FR-2.4 | Supports streaming (start playing before full audio received) | P1 |

### FR-3: Playback Control

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-3.1 | Agent can interrupt avatar playback (clear buffer) | P0 |
| FR-3.2 | Avatar notifies agent when playback starts | P1 |
| FR-3.3 | Avatar notifies agent when playback finishes | P0 |
| FR-3.4 | Playback position tracking for metrics | P2 |

### FR-4: Provider Support

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-4.1 | Support Tavus avatar provider | P0 |
| FR-4.2 | Support Anam avatar provider | P1 |
| FR-4.3 | Support Simli avatar provider | P1 |
| FR-4.4 | Support D-ID avatar provider | P2 |
| FR-4.5 | Pluggable architecture for additional providers | P0 |

### FR-5: Configuration

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-5.1 | Provider selection via environment variable | P0 |
| FR-5.2 | API key configuration per provider | P0 |
| FR-5.3 | Avatar identity/appearance selection | P1 |
| FR-5.4 | Sample rate configuration | P1 |

## Non-Functional Requirements

### NFR-1: Performance

| ID | Requirement | Target |
|----|-------------|--------|
| NFR-1.1 | Lip-sync latency (audio→video) | <300ms |
| NFR-1.2 | Avatar join time | <5s |
| NFR-1.3 | Interruption response time | <200ms |
| NFR-1.4 | Memory overhead per avatar | <50MB |

### NFR-2: Reliability

| ID | Requirement | Target |
|----|-------------|--------|
| NFR-2.1 | Avatar uptime during meeting | 99.9% |
| NFR-2.2 | Graceful degradation on provider failure | Required |
| NFR-2.3 | No audio loss on avatar failure | Required |

### NFR-3: Security

| ID | Requirement | Target |
|----|-------------|--------|
| NFR-3.1 | API keys not exposed in logs | Required |
| NFR-3.2 | JWT tokens for avatar room join | Required |
| NFR-3.3 | Short-lived tokens (<5 min) | Required |

## Success Metrics

| Metric | Baseline | Target | Measurement |
|--------|----------|--------|-------------|
| Developer adoption | 0 | 50 projects/month | GitHub stars, go.mod references |
| Lip-sync latency | N/A | <300ms P95 | Provider metrics |
| User engagement | N/A | +20% meeting duration | A/B testing |
| Bug reports | N/A | <2/month | GitHub issues |

## Competitive Analysis

| Feature | LiveKit Python | LiveKit JS | omni-livekit (proposed) |
|---------|----------------|------------|------------------------|
| Lip-sync avatars | Yes | Yes | **Planned** |
| Providers | 14 | 8 | 4 (initial) |
| Language | Python | TypeScript | **Go** |
| Static image avatar | Yes | Yes | Yes (existing) |
| Local avatar generation | No | No | No |

## Dependencies

### External Dependencies

| Dependency | Type | Risk |
|------------|------|------|
| Avatar providers (Tavus, Anam, etc.) | API | Medium - provider outages |
| LiveKit server | Infrastructure | Low - existing dependency |
| LiveKit Go SDK | Library | Low - existing dependency |

### Internal Dependencies

| Dependency | Type | Risk |
|------------|------|------|
| omni-livekit agent | Library | None - existing code |
| ByteStream support in Go SDK | Feature | Medium - may need contribution |

## Risks and Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Provider API changes | High | Medium | Abstract via interface, version pin |
| LiveKit Go SDK lacks ByteStream | High | Medium | Implement or contribute upstream |
| Latency too high for conversation | High | Low | Provider selection, optimization |
| Provider costs exceed budget | Medium | Medium | Usage tracking, alerts |

## Open Questions

1. **ByteStream in Go SDK**: Does the LiveKit Go SDK support ByteStream, or do we need to implement it?

2. **RPC in Go SDK**: Does the LiveKit Go SDK support participant RPC, or is this a gap?

3. **Provider pricing**: What are the per-minute costs for each avatar provider?

4. **Avatar customization**: How much avatar customization do users need (custom faces, voices, etc.)?

## Appendix

### A. Avatar Provider Comparison

| Provider | Latency | Quality | Price | API Complexity |
|----------|---------|---------|-------|----------------|
| Tavus | ~200ms | High | $$$ | Medium |
| Anam | ~150ms | High | $$ | Low |
| Simli | ~100ms | Medium | $$ | Low |
| D-ID | ~300ms | High | $$$ | Medium |
| HeyGen | ~250ms | High | $$$ | High |

### B. Related Documents

- [TRD.md](TRD.md) - Technical Requirements Document
- [PLAN.md](PLAN.md) - Implementation Plan
- [ROADMAP.md](ROADMAP.md) - Feature Roadmap
- [Static Image Avatar](../../architecture/avatar.md) - Existing static avatar docs
