package proxy

import "sync"

// FlowStore is a thread-safe, fixed-capacity ring buffer of flows with pub/sub.
type FlowStore struct {
	mu          sync.RWMutex
	flows       []*Flow
	index       map[string]*Flow
	capacity    int
	head        int // next write position
	count       int // current number of stored flows
	subscribers []chan FlowEvent
}

// NewFlowStore creates a store with the given capacity. Oldest flows are evicted when full.
func NewFlowStore(capacity int) *FlowStore {
	if capacity <= 0 {
		capacity = 1000
	}
	return &FlowStore{
		flows:    make([]*Flow, capacity),
		index:    make(map[string]*Flow),
		capacity: capacity,
	}
}

// Add stores a new flow and notifies subscribers.
func (s *FlowStore) Add(f *Flow) {
	s.mu.Lock()
	if s.count == s.capacity {
		// Evict the oldest entry.
		old := s.flows[s.head]
		if old != nil {
			delete(s.index, old.ID)
		}
	} else {
		s.count++
	}
	s.flows[s.head] = f
	s.index[f.ID] = f
	s.head = (s.head + 1) % s.capacity
	subs := s.copySubscribers()
	s.mu.Unlock()

	s.broadcast(subs, FlowEvent{Type: FlowEventNew, Flow: f})
}

// Update notifies subscribers of a change to an existing flow.
func (s *FlowStore) Update(f *Flow, eventType FlowEventType) {
	s.mu.RLock()
	subs := s.copySubscribers()
	s.mu.RUnlock()
	s.broadcast(subs, FlowEvent{Type: eventType, Flow: f})
}

// Get returns the flow with the given ID, or nil if not found.
func (s *FlowStore) Get(id string) *Flow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.index[id]
}

// All returns flows in insertion order (oldest first).
func (s *FlowStore) All() []*Flow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.count == 0 {
		return nil
	}
	result := make([]*Flow, 0, s.count)
	if s.count < s.capacity {
		for i := 0; i < s.count; i++ {
			if s.flows[i] != nil {
				result = append(result, s.flows[i])
			}
		}
	} else {
		for i := 0; i < s.capacity; i++ {
			idx := (s.head + i) % s.capacity
			if s.flows[idx] != nil {
				result = append(result, s.flows[idx])
			}
		}
	}
	return result
}

// Clear removes all flows from the store.
func (s *FlowStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flows = make([]*Flow, s.capacity)
	s.index = make(map[string]*Flow)
	s.head = 0
	s.count = 0
}

// Count returns the number of flows currently held.
func (s *FlowStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.count
}

// Subscribe returns a channel that receives FlowEvents. The channel is
// buffered; slow consumers will have events dropped.
func (s *FlowStore) Subscribe() chan FlowEvent {
	ch := make(chan FlowEvent, 128)
	s.mu.Lock()
	s.subscribers = append(s.subscribers, ch)
	s.mu.Unlock()
	return ch
}

// Unsubscribe removes and closes a subscription channel.
func (s *FlowStore) Unsubscribe(ch chan FlowEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, sub := range s.subscribers {
		if sub == ch {
			s.subscribers = append(s.subscribers[:i], s.subscribers[i+1:]...)
			close(ch)
			return
		}
	}
}

// copySubscribers returns a snapshot of the current subscriber list.
// Must be called with at least a read lock held.
func (s *FlowStore) copySubscribers() []chan FlowEvent {
	cp := make([]chan FlowEvent, len(s.subscribers))
	copy(cp, s.subscribers)
	return cp
}

func (s *FlowStore) broadcast(subs []chan FlowEvent, evt FlowEvent) {
	for _, ch := range subs {
		select {
		case ch <- evt:
		default:
			// Slow subscriber; drop the event rather than blocking.
		}
	}
}
