package kv

import (
	"context"
	"fmt"
	"iter"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/vfs"
	"sync"
)

type Pebbledb struct {
	db *pebble.DB

	// FIXME pebble doesnt actually the same MVCC as tikv. this is good enough for local testing
	globalWriteLock sync.Mutex
}

type PebbleWrite struct {
	p        *Pebbledb
	batch    *pebble.Batch
	db       *pebble.DB
	err      error
	commited bool
	locked   bool
}

func (w *PebbleWrite) Commit(ctx context.Context) error {
	if w.locked {
		w.locked = false
		w.p.globalWriteLock.Unlock()
	}
	if w.err != nil {
		return w.err
	}
	if w.commited {
		return fmt.Errorf("already committed")
	}
	err := w.batch.Commit(pebble.Sync)
	if err != nil {
		w.err = err
		return err
	}
	w.commited = true
	return nil
}

func (w *PebbleWrite) Rollback() error {
	if w.locked {
		w.locked = false
		w.p.globalWriteLock.Unlock()
	}
	if w.commited {
		return fmt.Errorf("already committed")
	}
	if w.err != nil {
		return w.err
	}
	return w.batch.Close()
}

func (w *PebbleWrite) Put(key []byte, value []byte) error {
	if !w.locked {
		w.p.globalWriteLock.Lock()
		w.locked = true
	}
	if w.err != nil {
		return w.err
	}
	err := w.batch.Set(key, value, pebble.Sync)
	if err != nil {
		w.Rollback()
		w.err = err
	}
	log.Debug("[pebble].Put:", "key", string(key), "err", err)
	return w.err
}

func (w *PebbleWrite) Get(ctx context.Context, key []byte) ([]byte, error) {
	if !w.locked {
		w.p.globalWriteLock.Lock()
		w.locked = true
	}
	if w.err != nil {
		return nil, w.err
	}

	val, closer, err := w.batch.Get(key)
	if err != nil {
		if err == pebble.ErrNotFound {
			log.Debug("[pebble].Get:", "key", string(key), "err", "not found")
			return nil, nil
		}
		log.Debug("[pebble].Get:", "key", string(key), "err", err)
		return nil, err
	}
	defer closer.Close()

	// Copy the value since the closer will invalidate it
	result := make([]byte, len(val))
	copy(result, val)

	log.Debug("[pebble].Get:", "key", string(key))
	return result, nil
}

func (w *PebbleWrite) Del(key []byte) error {
	if !w.locked {
		w.p.globalWriteLock.Lock()
		w.locked = true
	}
	if w.err != nil {
		return w.err
	}
	err := w.batch.Delete(key, pebble.Sync)
	if err != nil {
		w.Rollback()
		w.err = err
	}
	return w.err
}

func (r *PebbleWrite) Iter(ctx context.Context, start []byte, end []byte) iter.Seq2[KeyAndValue, error] {
	if !r.locked {
		r.p.globalWriteLock.Lock()
		r.locked = true
	}
	return func(yield func(KeyAndValue, error) bool) {
		iterOptions := &pebble.IterOptions{
			LowerBound: start,
			UpperBound: end,
		}
		it, err := r.batch.NewIter(iterOptions)
		if err != nil {
			yield(KeyAndValue{}, err)
			return
		}
		defer it.Close()

		for it.First(); it.Valid(); it.Next() {
			// Copy key and value since they may be invalidated by iterator movement
			key := append([]byte(nil), it.Key()...)
			val := append([]byte(nil), it.Value()...)

			log.Debug("[pebble].Iter:", "start", string(start), "end", string(end), "at", string(key))
			if !yield(KeyAndValue{K: key, V: val}, nil) {
				return
			}
		}

		if err := it.Error(); err != nil {
			log.Debug("[pebble].Iter:", "start", string(start), "end", string(end), "err", err)
			yield(KeyAndValue{}, err)
		}
	}
}

func (r *PebbleWrite) Close() {
	r.Rollback()
}

type PebbleRead struct {
	snapshot *pebble.Snapshot
	db       *pebble.DB
	err      error
}

func (r *PebbleRead) Get(ctx context.Context, key []byte) ([]byte, error) {
	if r.err != nil {
		return nil, r.err
	}

	val, closer, err := r.snapshot.Get(key)
	if err != nil {
		if err == pebble.ErrNotFound {
			log.Debug("[pebble].Get:", "key", string(key), "err", "not found")
			return nil, nil
		}
		log.Debug("[pebble].Get:", "key", string(key), "err", err)
		return nil, err
	}
	defer closer.Close()

	// Copy the value since the closer will invalidate it
	result := make([]byte, len(val))
	copy(result, val)

	log.Debug("[pebble].Get:", "key", string(key))
	return result, nil
}

func (r *PebbleRead) Close() {
	r.snapshot.Close()
}

func (r *PebbleRead) SetKeyOnly(b bool) {
	// Not supported in Pebble
}

func (r *PebbleRead) Iter(ctx context.Context, start []byte, end []byte) iter.Seq2[KeyAndValue, error] {
	return func(yield func(KeyAndValue, error) bool) {
		iterOptions := &pebble.IterOptions{
			LowerBound: start,
			UpperBound: end,
		}
		it, err := r.snapshot.NewIter(iterOptions)
		if err != nil {
			yield(KeyAndValue{}, err)
			return
		}

		for it.First(); it.Valid(); it.Next() {
			// Copy key and value since they may be invalidated by iterator movement
			key := append([]byte(nil), it.Key()...)
			val := append([]byte(nil), it.Value()...)

			log.Debug("[pebble].Iter:", "start", string(start), "end", string(end), "at", string(key))
			if !yield(KeyAndValue{K: key, V: val}, nil) {
				return
			}
		}

		if err := it.Error(); err != nil {
			log.Debug("[pebble].Iter:", "start", string(start), "end", string(end), "err", err)
			yield(KeyAndValue{}, err)
		}
	}
}

func (p *Pebbledb) Close() {
	p.db.Close()
}

func (p *Pebbledb) Write() Write {
	batch := p.db.NewIndexedBatch()
	return &PebbleWrite{p: p, batch: batch, db: p.db, err: nil, commited: false}
}

func (p *Pebbledb) Read() Read {
	snapshot := p.db.NewSnapshot()
	return &PebbleRead{snapshot: snapshot, db: p.db, err: nil}
}

func NewPebble() (KV, error) {
	opts := &pebble.Options{
		// Configure any needed options here
	}

	db, err := pebble.Open("pebble-db", opts)
	if err != nil {
		return nil, err
	}

	return &Pebbledb{db: db}, nil
}

// NewPebbleInMem creates a new in-memory Pebble database instance for testing.
func NewMemPebble() (KV, error) {
	opts := &pebble.Options{
		FS: vfs.NewMem(),
	}

	db, err := pebble.Open("", opts)
	if err != nil {
		return nil, err
	}

	return &Pebbledb{db: db}, nil
}

