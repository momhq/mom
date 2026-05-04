package herald

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/momhq/mom/cli/internal/archtest"
)

// These tests lock the v0.30 Herald contract on top of the existing v1
// Bus surface: type-only routing, unsubscribe semantics, panic isolation,
// and the architectural rule that Herald has no Vault/Librarian dependency.

func TestPublish_TypeOnlyRouting(t *testing.T) {
	bus := NewBus()
	var aCount, bCount atomic.Int64

	bus.Subscribe(SessionStart, func(e Event) { aCount.Add(1) })
	bus.Subscribe(SessionEnd, func(e Event) { bCount.Add(1) })

	bus.Publish(Event{Type: SessionStart})
	bus.Publish(Event{Type: SessionStart})
	bus.Publish(Event{Type: SessionEnd})

	if got := aCount.Load(); got != 2 {
		t.Errorf("SessionStart handler got %d events, want 2", got)
	}
	if got := bCount.Load(); got != 1 {
		t.Errorf("SessionEnd handler got %d events, want 1", got)
	}
}

func TestSubscribe_ReturnsUnsubscribe_StopsDelivery(t *testing.T) {
	bus := NewBus()
	var count atomic.Int64

	unsub := bus.Subscribe(MemoryCreated, func(e Event) { count.Add(1) })

	bus.Publish(Event{Type: MemoryCreated})
	if got := count.Load(); got != 1 {
		t.Fatalf("got %d, want 1 (before unsubscribe)", got)
	}

	unsub()

	bus.Publish(Event{Type: MemoryCreated})
	if got := count.Load(); got != 1 {
		t.Errorf("got %d, want 1 (handler should not fire after unsubscribe)", got)
	}
}

func TestUnsubscribe_IsIdempotent(t *testing.T) {
	bus := NewBus()
	var count atomic.Int64

	unsub := bus.Subscribe(ToolUse, func(e Event) { count.Add(1) })

	// Calling unsubscribe twice must not panic and must not affect other
	// subscribers registered for the same type.
	bus.Subscribe(ToolUse, func(e Event) { count.Add(10) })

	unsub()
	unsub() // second call is a no-op

	bus.Publish(Event{Type: ToolUse})
	if got := count.Load(); got != 10 {
		t.Errorf("got %d, want 10 (only the still-subscribed handler should fire)", got)
	}
}

func TestUnsubscribe_OnlyAffectsTheReturnedHandler(t *testing.T) {
	bus := NewBus()
	var aCount, bCount atomic.Int64

	unsubA := bus.Subscribe(MemoryPromoted, func(e Event) { aCount.Add(1) })
	bus.Subscribe(MemoryPromoted, func(e Event) { bCount.Add(1) })

	unsubA()
	bus.Publish(Event{Type: MemoryPromoted})

	if a := aCount.Load(); a != 0 {
		t.Errorf("unsubscribed handler fired %d times", a)
	}
	if b := bCount.Load(); b != 1 {
		t.Errorf("other handler fired %d times, want 1", b)
	}
}

func TestPublish_HandlerPanicDoesNotBlockOthers(t *testing.T) {
	bus := NewBus()
	var beforeCount, afterCount atomic.Int64

	bus.Subscribe(Error, func(e Event) { beforeCount.Add(1) })
	bus.Subscribe(Error, func(e Event) { panic("handler exploded") })
	bus.Subscribe(Error, func(e Event) { afterCount.Add(1) })

	// Publish must not propagate the panic and must call handlers
	// registered after the panicking one.
	bus.Publish(Event{Type: Error, Payload: map[string]any{"msg": "test"}})

	if got := beforeCount.Load(); got != 1 {
		t.Errorf("before-panic handler got %d, want 1", got)
	}
	if got := afterCount.Load(); got != 1 {
		t.Errorf("after-panic handler got %d, want 1 — fan-out was blocked by the panic", got)
	}
}

func TestPublish_HandlerPanicAcrossMultiplePublishes(t *testing.T) {
	// A panicking handler should not deregister itself or break the bus
	// for future publishes.
	bus := NewBus()
	var fireCount atomic.Int64
	bus.Subscribe(ConfigChanged, func(e Event) {
		fireCount.Add(1)
		panic("boom")
	})

	for i := 0; i < 5; i++ {
		bus.Publish(Event{Type: ConfigChanged})
	}

	if got := fireCount.Load(); got != 5 {
		t.Errorf("handler fired %d times, want 5 (panic must not deregister it)", got)
	}
}

func TestSubscribe_ConcurrentUnsubscribeIsRaceFree(t *testing.T) {
	bus := NewBus()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unsub := bus.Subscribe(TurnComplete, func(e Event) {})
			unsub()
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Publish(Event{Type: TurnComplete})
		}()
	}
	wg.Wait()
}

// TestHerald_NoVaultOrLibrarianDependency enforces PRD 0003: Herald
// is a pure pub/sub bus and must not import Vault or Librarian.
// Herald's only imports are stdlib, so a direct-import check is
// sufficient — there's no transitive path in production code.
func TestHerald_NoVaultOrLibrarianDependency(t *testing.T) {
	archtest.AssertNoDirectImport(t, ".",
		"github.com/momhq/mom/cli/internal/vault",
		"github.com/momhq/mom/cli/internal/librarian",
	)
}
