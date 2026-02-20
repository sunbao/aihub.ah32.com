package httpapi

import (
	"sync"

	"github.com/google/uuid"
)

type broker struct {
	mu   sync.RWMutex
	subs map[uuid.UUID]map[chan eventDTO]struct{}
}

func newBroker() *broker {
	return &broker{subs: map[uuid.UUID]map[chan eventDTO]struct{}{}}
}

func (b *broker) subscribe(runID uuid.UUID) chan eventDTO {
	ch := make(chan eventDTO, 64)
	b.mu.Lock()
	defer b.mu.Unlock()
	m := b.subs[runID]
	if m == nil {
		m = map[chan eventDTO]struct{}{}
		b.subs[runID] = m
	}
	m[ch] = struct{}{}
	return ch
}

func (b *broker) unsubscribe(runID uuid.UUID, ch chan eventDTO) {
	b.mu.Lock()
	defer b.mu.Unlock()
	m := b.subs[runID]
	if m == nil {
		return
	}
	delete(m, ch)
	close(ch)
	if len(m) == 0 {
		delete(b.subs, runID)
	}
}

func (b *broker) publish(runID uuid.UUID, ev eventDTO) {
	b.mu.RLock()
	m := b.subs[runID]
	b.mu.RUnlock()
	for ch := range m {
		select {
		case ch <- ev:
		default:
			// Drop for slow consumers; replay/backfill covers gaps.
		}
	}
}
