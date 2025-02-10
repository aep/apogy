package stream

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type SoloLock struct {
	ctx context.Context

	expectedKeepAlive time.Duration
	lastKeepAlive     time.Time
	cancel            func()
	fKeepalive        func() error
	fUnlock           func()
}

func (self *SoloLock) Deadline() (deadline time.Time, ok bool) {
	return self.ctx.Deadline()
}

func (self *SoloLock) Done() <-chan struct{} {
	return self.ctx.Done()
}

func (self *SoloLock) Err() error {
	return self.ctx.Err()
}

func (self *SoloLock) Value(key any) any {
	return self.ctx.Value(key)
}

func (self *SoloLock) KeepAlive() error {
	return self.fKeepalive()
}

func (self *SoloLock) Unlock() {
	self.fUnlock()
}

type SoloBus struct {
	m     sync.Mutex
	locks map[string]*SoloLock
	subs  map[string]chan []byte
}

func (s *SoloBus) Lock(ctx context.Context, key string, ka time.Duration) (Lock, error) {
	s.m.Lock()
	defer s.m.Unlock()

	if s.locks[key] != nil {
		return nil, fmt.Errorf("locked")
	}

	ctx, cancel := context.WithCancel(ctx)
	sl := &SoloLock{
		expectedKeepAlive: ka,
		lastKeepAlive:     time.Now(),
		ctx:               ctx,
		cancel:            cancel,
		fUnlock: func() {
			s.m.Lock()
			if s.locks[key] != nil {
				defer s.locks[key].cancel()
			}
			delete(s.locks, key)
			s.m.Unlock()
		},
		fKeepalive: func() error {
			s.m.Lock()
			defer s.m.Unlock()

			if s.locks[key] == nil {
				return fmt.Errorf("not locked")
			}
			s.locks[key].lastKeepAlive = time.Now()
			return nil
		},
	}
	s.locks[key] = sl

	return sl, nil
}

func (self *SoloBus) Send(topic string, v []byte) error {

	self.m.Lock()
	defer self.m.Unlock()

	if self.subs[topic] != nil {
		select {
		case self.subs[topic] <- v:
		default:
		}
	}

	return nil
}

func (self *SoloBus) Recv(topic string) chan []byte {

	self.m.Lock()
	defer self.m.Unlock()

	if self.subs[topic] == nil {
		self.subs[topic] = make(chan []byte)
	}

	return self.subs[topic]
}

func NewSolo() (Bus, error) {

	self := &SoloBus{
		locks: make(map[string]*SoloLock),
		subs:  make(map[string]chan []byte),
	}

	go func() {
		t := time.NewTicker(time.Millisecond * 200)
		defer t.Stop()
		for range t.C {
			self.m.Lock()
			for k, v := range self.locks {
				if time.Since(v.lastKeepAlive) > v.expectedKeepAlive {
					v.cancel()
					delete(self.locks, k)
				}
			}
			self.m.Unlock()
		}
	}()

	return self, nil
}
