package main

import "sync"

type Bus struct {
	mu          sync.RWMutex
	subscribers map[string][]chan *Envelop
}

func NewBus() *Bus {
	return &Bus{
		subscribers: make(map[string][]chan *Envelop),
	}
}

func (b *Bus) Subscribe(channel string) chan *Envelop {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan *Envelop, 100)
	b.subscribers[channel] = append(b.subscribers[channel], ch)
	return ch
}

func (b *Bus) Unsubscribe(channel string, ch chan *Envelop) {
	b.mu.Lock()
	defer b.mu.Unlock()

	chs := b.subscribers[channel]
	for i, c := range chs {
		if c == ch {
			b.subscribers[channel] = append(chs[:i], chs[i+1:]...)
			break
		}
	}
}

func (b *Bus) Publish(channel string, env *Envelop) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if channel == "" {
		return
	}
	for _, ch := range b.subscribers[channel] {
		select {
		case ch <- env.Clone():
		default:
		}
	}
}
