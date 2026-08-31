package throttling

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeClock lets tests move time forward deterministically instead of sleeping.
type fakeClock struct {
	t time.Time
}

func (c *fakeClock) now() time.Time {
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.t = c.t.Add(d)
}

func newTestThrottler() (*ThrottlingSyncMap, *fakeClock) {
	tsm := NewSyncMapThrottler()
	clock := &fakeClock{t: time.Now()}
	tsm.store.now = clock.now
	return tsm, clock
}

func TestShouldRun_WindowSuppressionAndExpiry(t *testing.T) {
	tsm, clock := newTestThrottler()
	settings := &ThrottlingSettings{Interval: 10 * time.Minute, DisableRandomPreSetInterval: true}

	assert.True(t, tsm.ShouldRun("k", settings), "first call with no prior entry should run")
	assert.False(t, tsm.ShouldRun("k", settings), "second call within the window should be suppressed")

	clock.advance(5 * time.Minute)
	assert.False(t, tsm.ShouldRun("k", settings), "still within the window")

	clock.advance(6 * time.Minute)
	assert.True(t, tsm.ShouldRun("k", settings), "past the window should run again")
}

func TestShouldRun_DistinctKeysDoNotInterfere(t *testing.T) {
	tsm, _ := newTestThrottler()
	settings := &ThrottlingSettings{Interval: 10 * time.Minute, DisableRandomPreSetInterval: true}

	assert.True(t, tsm.ShouldRun("a", settings))
	assert.False(t, tsm.ShouldRun("a", settings))

	// "b" has never run, so it must not be suppressed by "a"'s state.
	assert.True(t, tsm.ShouldRun("b", settings))
}

func TestShouldRun_DistinctPartitionsDoNotInterfere(t *testing.T) {
	tsm, _ := newTestThrottler()
	settings := &ThrottlingSettings{Interval: 10 * time.Minute, DisableRandomPreSetInterval: true}

	p1 := tsm.WithPartition("p1")
	p2 := tsm.WithPartition("p2")

	assert.True(t, p1.ShouldRun("k", settings))
	assert.False(t, p1.ShouldRun("k", settings))

	// same key, different partition: must be independent of p1's state
	assert.True(t, p2.ShouldRun("k", settings))
}

func TestShouldRun_RandomPreSetInterval(t *testing.T) {
	settings := &ThrottlingSettings{Interval: 10 * time.Minute}

	t.Run("enabled by default seeds and suppresses the first call", func(t *testing.T) {
		tsm, _ := newTestThrottler()
		assert.False(t, tsm.ShouldRun("k", settings), "first call should be pre-seeded and suppressed")
	})

	t.Run("DisableRandomPreSetInterval true runs the first call", func(t *testing.T) {
		tsm, _ := newTestThrottler()
		disabled := &ThrottlingSettings{Interval: 10 * time.Minute, DisableRandomPreSetInterval: true}
		assert.True(t, tsm.ShouldRun("k", disabled), "first call should run when pre-seeding is disabled")
	})
}

func TestEviction_HappensAfterTTLViaInjectedClock(t *testing.T) {
	tsm, clock := newTestThrottler()
	settings := &ThrottlingSettings{Interval: 5 * time.Minute, DisableRandomPreSetInterval: true}

	tsm.ShouldRun("k", settings)
	_, ok := tsm.store.syncMap.Load(":k")
	require.True(t, ok, "entry should exist right after the call")

	// Push past the 1h TTL and the sweep gate, then trigger a sweep via any
	// ShouldRun call on an unrelated key.
	clock.advance(time.Hour + time.Minute)
	tsm.ShouldRun("other", settings)

	_, ok = tsm.store.syncMap.Load(":k")
	assert.False(t, ok, "idle entry should have been swept")
}

func TestEviction_EnsureStepSuccessKeyReadEveryIntervalIsNotEvicted(t *testing.T) {
	// A step with EnsureStepSuccess never calls SetNow while it keeps failing, so its
	// stamp ages without bound even though it's read every reconcile. Eviction must key
	// off last access, not the stamp, or this hot key would be swept as if idle.
	tsm, clock := newTestThrottler()
	settings := &ThrottlingSettings{Interval: time.Minute, EnsureStepSuccess: true}

	// First call seeds the entry via the random pre-set interval (that still happens
	// with EnsureStepSuccess; only the later SetNow-on-success is gated by it).
	tsm.ShouldRun("k", settings)
	_, ok := tsm.store.syncMap.Load(":k")
	require.True(t, ok, "first call should have seeded an entry")

	// Keep reading it well past the eviction TTL a stamp-age sweep would use, while
	// never letting it go anywhere near that TTL between accesses.
	for i := 0; i < 40; i++ {
		clock.advance(2 * time.Minute)
		result := tsm.ShouldRun("k", settings)
		assert.True(t, result, "stale + EnsureStepSuccess should keep returning true without refreshing the stamp")

		_, ok := tsm.store.syncMap.Load(":k")
		require.True(t, ok, "key must survive: it's read continuously, so it's never idle")
	}
}

func TestReset_IsNotSweptAsAncient(t *testing.T) {
	tsm, clock := newTestThrottler()
	settings := &ThrottlingSettings{Interval: time.Minute, DisableRandomPreSetInterval: true}

	tsm.ShouldRun("k", settings)
	tsm.Reset("k")

	entry, ok := tsm.store.syncMap.Load(":k")
	require.True(t, ok)
	assert.True(t, entry.stamp.IsZero(), "Reset stores a zero stamp meaning 'run now'")

	// A sweep keyed off the (zero) stamp would treat this as infinitely old and evict
	// it immediately. Advance well past the sweep gate but within the 1h eviction
	// TTL, then sweep. Reset refreshed last-access, so a last-access-based sweep
	// must let it survive.
	clock.advance(30 * time.Minute)
	tsm.ShouldRun("other", settings)

	_, ok = tsm.store.syncMap.Load(":k")
	assert.True(t, ok, "a just-Reset entry must not be swept")

	assert.True(t, tsm.ShouldRun("k", settings), "Reset should allow it to run immediately")
}

func TestWithPartition_SharesOneStoreAcrossPartitions(t *testing.T) {
	tsm, clock := newTestThrottler()
	settings := &ThrottlingSettings{Interval: time.Minute, DisableRandomPreSetInterval: true}

	p1 := tsm.WithPartition("p1")
	p2 := tsm.WithPartition("p2")

	p1.ShouldRun("k", settings)

	// A sweep triggered through p2 must see (and be able to evict) entries written
	// through p1, since WithPartition only changes the key prefix, not the store.
	clock.advance(2 * time.Hour)
	p2.ShouldRun("unrelated", settings)

	_, ok := tsm.store.syncMap.Load("p1:k")
	assert.False(t, ok, "sweep triggered via p2 should have evicted p1's idle entry")
}
