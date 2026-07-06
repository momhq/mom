package projection

import (
	"context"
	"testing"
	"time"
)

// countingSynth is a trivial synthesizer that counts Fold calls; FoldAll's
// chunk-cache behavior is what's under test, not the synthesis itself.
type countingSynth struct {
	calls int
}

func (c *countingSynth) Fold(_ context.Context, _ FoldInput) (FoldResult, error) {
	c.calls++
	return FoldResult{Files: map[string]string{}}, nil
}

func TestFoldAllIdempotency(t *testing.T) {
	events := []FoldEvent{
		makeMemoryEvent(1, "s1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "event 1", nil),
		makeMemoryEvent(2, "s1", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), "event 2", nil),
	}

	synth := &countingSynth{}
	in := FoldInput{
		ProjectID: "test-proj",
		Events:    events,
	}

	// First fold.
	res1, err := FoldAll(context.Background(), synth, in, 10)
	if err != nil {
		t.Fatalf("first fold error: %v", err)
	}
	firstCalls := synth.calls

	// Second fold with ExistingChunks from first result.
	synth.calls = 0
	in2 := FoldInput{
		ProjectID:      "test-proj",
		Events:         events,
		Existing:       res1.Files,
		ExistingChunks: res1.Chunks,
	}
	_, err = FoldAll(context.Background(), synth, in2, 10)
	if err != nil {
		t.Fatalf("second fold error: %v", err)
	}

	if synth.calls >= firstCalls {
		t.Errorf("second fold should skip cached chunks; got %d calls (first had %d)", synth.calls, firstCalls)
	}
}

func TestFoldAllRebuildSkipsNoChunks(t *testing.T) {
	events := []FoldEvent{
		makeMemoryEvent(1, "s1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "event 1", nil),
	}
	synth := &countingSynth{}

	// First fold to get chunk map.
	res1, _ := FoldAll(context.Background(), synth, FoldInput{ProjectID: "proj", Events: events}, 10)

	// Rebuild: ExistingChunks is nil.
	synth.calls = 0
	_, err := FoldAll(context.Background(), synth, FoldInput{
		ProjectID:      "proj",
		Events:         events,
		Existing:       res1.Files,
		ExistingChunks: nil, // nil = rebuild, no skipping
	}, 10)
	if err != nil {
		t.Fatalf("rebuild error: %v", err)
	}
	if synth.calls == 0 {
		t.Error("rebuild should synthesize all chunks, not skip them")
	}
}
