package throttling

import (
	"sync"
)

type TypedSyncMap[K comparable, V any] struct {
	m sync.Map
}

func (tsm *TypedSyncMap[K, V]) Store(key K, value V) {
	tsm.m.Store(key, value)
}

func (tsm *TypedSyncMap[K, V]) Load(key K) (V, bool) {
	value, ok := tsm.m.Load(key)
	if !ok {
		var zero V
		return zero, false
	}
	return value.(V), true
}

func (tsm *TypedSyncMap[K, V]) LoadOrStore(key K, value V) (V, bool) {
	actual, loaded := tsm.m.LoadOrStore(key, value)
	if loaded {
		return actual.(V), true
	}
	return value, false
}

func (tsm *TypedSyncMap[K, V]) Delete(key K) {
	tsm.m.Delete(key)
}

func (tsm *TypedSyncMap[K, V]) Range(f func(K, V) bool) {
	tsm.m.Range(func(key, value any) bool {
		return f(key.(K), value.(V))
	})
}
