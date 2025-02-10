package stream

import (
	"context"
	"time"
)

type Lock interface {
	context.Context
	Unlock()
	KeepAlive() error
}

type Bus interface {
	Lock(ctx context.Context, key string, ka time.Duration) (Lock, error)

	Send(k string, v []byte) error
	Recv(topic string) chan []byte
}
