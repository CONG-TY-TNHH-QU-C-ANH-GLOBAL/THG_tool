package reel

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Deterministic, zero-cost fakes for the enriched pipeline ports. These are
// the dev/CI defaults until the real adapters land (PR-E3..E7): no external
// I/O, no spend, no failure modes. Kept in the service package so the wiring
// and tests can use them without a separate module.

// FakeObjectStore is an in-memory ObjectStore. Put keeps bytes in a map;
// SignedURL returns a stable mem:// URL. Safe for concurrent use.
type FakeObjectStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

// NewFakeObjectStore returns an empty in-memory object store.
func NewFakeObjectStore() *FakeObjectStore {
	return &FakeObjectStore{objects: map[string][]byte{}}
}

// Put stores the reader's bytes under key.
func (f *FakeObjectStore) Put(_ context.Context, key string, r io.Reader, _ string) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = b
	return nil
}

// SignedURL returns a deterministic mem:// URL for key.
func (f *FakeObjectStore) SignedURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return "mem://" + key, nil
}

// Has reports whether key was stored — for test assertions.
func (f *FakeObjectStore) Has(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.objects[key]
	return ok
}

// FakeTranscriber returns a fixed two-cue Vietnamese transcript, zero cost.
type FakeTranscriber struct{}

// Transcribe implements Transcriber.
func (FakeTranscriber) Transcribe(_ context.Context, _ string) (Transcript, error) {
	return Transcript{
		Cues: []Cue{
			{Text: "Xin chao", FromMS: 0, ToMS: 800},
			{Text: "cam on ban", FromMS: 800, ToMS: 1600},
		},
		LangSrc: "vi",
		Source:  "whisper",
		CostUSD: 0,
	}, nil
}

// FakeScriptEngine echoes the transcript cues as subtitles and derives a
// trivial avatar script — deterministic, zero cost, no invented facts.
type FakeScriptEngine struct{}

// Generate implements ScriptEngine.
func (FakeScriptEngine) Generate(_ context.Context, in ScriptInput) (EnrichedScript, error) {
	avatarLine := "Fake avatar script"
	if len(in.Transcript.Cues) > 0 {
		avatarLine = fmt.Sprintf("Fake avatar script over %d cues", len(in.Transcript.Cues))
	}
	return EnrichedScript{
		Subtitles:    in.Transcript.Cues,
		AvatarScript: avatarLine,
		LangTgt:      "en",
		CostUSD:      0,
	}, nil
}

// FakeAvatarRenderer writes a tiny placeholder file and returns its path, so
// the service's upload step exercises a real file. Zero cost.
type FakeAvatarRenderer struct{}

// RenderAvatar implements AvatarRenderer.
func (FakeAvatarRenderer) RenderAvatar(_ context.Context, req AvatarReq) (AvatarResult, error) {
	f, err := os.CreateTemp("", "fake-avatar-*.mp4")
	if err != nil {
		return AvatarResult{}, err
	}
	defer f.Close()
	if _, err := f.WriteString("fake-avatar:" + req.AvatarID); err != nil {
		return AvatarResult{}, err
	}
	return AvatarResult{TempPath: f.Name(), ContentType: "video/mp4", CostUSD: 0}, nil
}

// FakeComposer returns a deterministic final key without producing media.
// Zero cost.
type FakeComposer struct{}

// Compose implements Composer.
func (FakeComposer) Compose(_ context.Context, req ComposeReq) (ComposeResult, error) {
	return ComposeResult{FinalKey: "fake/final-" + req.AvatarPos + ".mp4", CostUSD: 0}, nil
}

// NewFakeEnrichedService builds an EnrichedService wired entirely with fakes
// — the one-call dev/CI constructor used by tests and the local PoC path.
// store is any EnrichedStore (the in-memory double locally, the Postgres
// store in production).
func NewFakeEnrichedService(store EnrichedStore, deps EnrichedDeps) *EnrichedService {
	return NewEnrichedService(store, NewFakeObjectStore(), FakeTranscriber{}, FakeScriptEngine{}, FakeAvatarRenderer{}, FakeComposer{}, deps)
}
