package kv

import (
	"context"
	"iter"
)

type KeyAndValue struct {
	K []byte
	V []byte
}

type KV interface {
	Close()
	Write() Write
	Read() Read
}

type Read interface {
	BatchGet(ctx context.Context, keys [][]byte) (map[string][]byte, error)
	Get(ctx context.Context, key []byte) ([]byte, error)
	Iter(ctx context.Context, srart []byte, end []byte) iter.Seq2[KeyAndValue, error]
	Close()
}

type Write interface {
	Read
	Get(ctx context.Context, key []byte) ([]byte, error)
	Put(key []byte, value []byte) error
	Del(key []byte) error
	Commit(ctx context.Context) error
	Rollback() error
	Close()
}
