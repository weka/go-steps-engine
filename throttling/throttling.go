package throttling

import (
	"math/rand/v2"
	"sync/atomic"
	"time"
)

type ThrottlingSettings struct {
	Interval time.Duration
	// select random time within interval for initial value,
	// so first time always will be trottled to distribute many such callers
	// (enabled by default)
	DisableRandomPreSetInterval bool
	// will update the timestamp on ThrottlingMap only if the step succeeded
	// (disabled by default)
	EnsureStepSuccess bool
	// (optional) override partition key for the throttler
	PartitionKeyOverride *string
}

// throttleEntry is what's stored per key. stamp is the throttling clock (zero
// means "run now", set by Reset). lastAccess is tracked separately because a key
// whose stamp is deliberately frozen - see ShouldRun's EnsureStepSuccess path -
// is still live and must not look idle to the sweep.
type throttleEntry struct {
	stamp      time.Time
	lastAccess time.Time
}

// evictionTTL is how long an entry may sit unread before the sweep drops it.
// Every ShouldRun refreshes lastAccess, so an entry only goes idle once its owner
// stops being reconciled at all, which is unrelated to the step's own Interval.
const evictionTTL = time.Hour

// evictionSweepInterval bounds how often a ShouldRun call pays for a sweep pass,
// and doubles as the granularity at which lastAccess is refreshed.
const evictionSweepInterval = time.Minute

// throttlingStore holds the map, sweep gate and clock shared by a
// ThrottlingSyncMap and every partition derived from it via WithPartition.
type throttlingStore struct {
	syncMap *TypedSyncMap[string, throttleEntry]

	// unix nanos of the last sweep; zero means never swept
	lastSweep atomic.Int64

	now func() time.Time
}

type ThrottlingSyncMap struct {
	store     *throttlingStore
	partition string
}

type Throttler interface {
	// Store stores the current time for the given key
	ShouldRun(key string, s *ThrottlingSettings) bool
	SetNow(key string)
	WithPartition(partition string) Throttler
	// Reset removes the stored time for the given key, allowing it to run immediately
	Reset(key string)
}

func NewSyncMapThrottler() *ThrottlingSyncMap {
	return &ThrottlingSyncMap{
		partition: "",
		store: &throttlingStore{
			syncMap: &TypedSyncMap[string, throttleEntry]{},
			now:     time.Now,
		},
	}
}

func (tsm *ThrottlingSyncMap) partKey(key string) string {
	return tsm.partition + ":" + key
}

// ShouldRun reports whether key may run now, where s.Interval defines how often
// that is allowed. It is also the only entry point that sweeps idle keys, so a
// store that never sees a ShouldRun call never evicts.
func (tsm *ThrottlingSyncMap) ShouldRun(key string, s *ThrottlingSettings) bool {
	tsm.sweep()

	partKey := tsm.partKey(key)
	now := tsm.store.now()

	if entry, ok := tsm.store.syncMap.Load(partKey); ok {
		stale := now.Sub(entry.stamp) > s.Interval

		switch {
		case stale && !s.EnsureStepSuccess:
			entry.stamp = now
			entry.lastAccess = now
			tsm.store.syncMap.Store(partKey, entry)
		case now.Sub(entry.lastAccess) >= evictionSweepInterval:
			// Only the sweep reads lastAccess, so refresh it at sweep granularity:
			// with EnsureStepSuccess the stamp advances only on an explicit SetNow, and
			// writing on every check would cost a map store per call on a hot key.
			entry.lastAccess = now
			tsm.store.syncMap.Store(partKey, entry)
		}

		return stale
	}

	if !s.DisableRandomPreSetInterval {
		milliSeconds := s.Interval.Milliseconds()
		randomPreSetInterval := time.Duration(rand.IntN(int(milliSeconds)))
		// even if some other time set in parallel - safe to assume it would not allow us to run
		tsm.store.syncMap.LoadOrStore(partKey, throttleEntry{stamp: now.Add(-randomPreSetInterval), lastAccess: now})

		return false
	}

	if !s.EnsureStepSuccess {
		tsm.setEntry(partKey, now)

		return true
	}

	// EnsureStepSuccess with pre-seeding disabled: nothing is recorded until an
	// explicit SetNow, so the first run is always allowed.
	return true
}

// setEntry stores a fresh stamp for an already-partitioned key.
func (tsm *ThrottlingSyncMap) setEntry(partKey string, now time.Time) {
	tsm.store.syncMap.Store(partKey, throttleEntry{stamp: now, lastAccess: now})
}

func (tsm *ThrottlingSyncMap) SetNow(key string) {
	tsm.setEntry(tsm.partKey(key), tsm.store.now())
}

func (tsm *ThrottlingSyncMap) Reset(key string) {
	// the zero stamp means "run now", not "ancient"; lastAccess is still refreshed
	// so the sweep never mistakes this sentinel for an idle key
	tsm.store.syncMap.Store(tsm.partKey(key), throttleEntry{lastAccess: tsm.store.now()})
}

func (tsm *ThrottlingSyncMap) WithPartition(partition string) Throttler {
	var newPartition string
	if tsm.partition != "" {
		newPartition = tsm.partition + ":" + partition
	} else {
		newPartition = partition
	}
	return &ThrottlingSyncMap{
		partition: newPartition,
		store:     tsm.store,
	}
}

// sweep evicts idle entries, at most once per evictionSweepInterval across the
// whole shared store (not per partition).
func (tsm *ThrottlingSyncMap) sweep() {
	store := tsm.store
	now := store.now()

	last := store.lastSweep.Load()
	if now.UnixNano()-last < int64(evictionSweepInterval) {
		return
	}
	// loser of a concurrent CAS leaves the sweep to the winner
	if !store.lastSweep.CompareAndSwap(last, now.UnixNano()) {
		return
	}

	store.syncMap.Range(func(key string, entry throttleEntry) bool {
		if now.Sub(entry.lastAccess) > evictionTTL {
			store.syncMap.Delete(key)
		}
		return true
	})
}
