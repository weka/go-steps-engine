package throttling

import (
	"math/rand/v2"
	"sync"
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

type ThrottlingSyncMap struct {
	syncMap   *TypedSyncMap[string, time.Time]
	partition string
}

type Throttler interface {
	// Store stores the current time for the given key
	ShouldRun(key string, s *ThrottlingSettings) bool
	SetNow(key string)
}

func NewSyncMapThrottler() *ThrottlingSyncMap {
	return &ThrottlingSyncMap{
		partition: "",
		syncMap: &TypedSyncMap[string, time.Time]{
			m: sync.Map{},
		},
	}
}

func (tsm *ThrottlingSyncMap) ShouldRun(key string, s *ThrottlingSettings) bool {
	// interval defines throttling interval, i.e how often sometihng should be allowed to be done
	partKey := tsm.partition + ":" + key

	if value, ok := tsm.syncMap.Load(partKey); ok {
		if time.Since(value) > s.Interval {
			if !s.EnsureStepSuccess {
				tsm.SetNow(key)
			}
			return true
		}
		return false
	} else if !s.DisableRandomPreSetInterval {
		milliSeconds := s.Interval.Milliseconds()
		randomPreSetInterval := time.Duration(rand.IntN(int(milliSeconds)))
		newTime := time.Now().Add(-randomPreSetInterval)
		// even if some other time set in parallel - safe to assume it would not allow us to run
		tsm.syncMap.LoadOrStore(partKey, newTime)

		return false
	} else if !s.EnsureStepSuccess {
		tsm.SetNow(key)

		return true
	}

	return true
}

func (tsm *ThrottlingSyncMap) SetNow(key string) {
	key = tsm.partition + ":" + key
	// SetNow stores the current time for the given key
	tsm.syncMap.Store(key, time.Now())
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
		syncMap:   tsm.syncMap,
	}
}
