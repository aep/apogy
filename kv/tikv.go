package kv

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"os"
	"time"

	pingcaplog "github.com/pingcap/log"

	"github.com/lmittmann/tint"
	tikverr "github.com/tikv/client-go/v2/error"
	"github.com/tikv/client-go/v2/kv"
	"github.com/tikv/client-go/v2/txnkv"
	"github.com/tikv/client-go/v2/txnkv/txnsnapshot"

	"go.opentelemetry.io/otel"
	//"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var tracer trace.Tracer

func init() {
	l, p, _ := pingcaplog.InitLogger(&pingcaplog.Config{})

	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	config.OutputPaths = []string{"stderr"}
	config.ErrorOutputPaths = []string{"stderr"}
	l, _ = config.Build()

	pingcaplog.ReplaceGlobals(l, p)

	tracer = otel.Tracer("github.com/aep/apogy/kv")
}

var log = slog.New(tint.NewHandler(os.Stderr, nil))

type Tikv struct {
	k *txnkv.Client
}

type TikvWrite struct {
	txn      *txnkv.KVTxn
	err      error
	commited bool

	statLockRetries int
}

func (w *TikvWrite) Stat() (statLockRetries int) {
	return w.statLockRetries
}

func (w *TikvWrite) Commit(ctx context.Context) error {
	if w.err != nil {
		return w.err
	}
	if w.commited {
		return fmt.Errorf("already commited")
	}

	ctx, span := tracer.Start(ctx, "kv.TikvWrite.Commit")
	defer span.End()

	err := w.txn.Commit(ctx)
	if err != nil {
		w.err = err
		return err
	}
	w.commited = true
	return nil
}

func (w *TikvWrite) Rollback() error {

	if w.commited {
		return fmt.Errorf("already commited")
	}
	if w.err != nil {
		return w.err
	}

	return w.txn.Rollback()
}

func (w *TikvWrite) Put(key []byte, value []byte) error {
	if w.err != nil {
		return w.err
	}
	err := w.txn.Set(key, value)
	if err != nil {
		w.Rollback()
		w.err = err
	}
	log.Debug("[tikv].Put:", "key", string(key), "err", err)
	return w.err
}

func (w *TikvWrite) Get(ctx context.Context, key []byte) ([]byte, error) {
	if w.err != nil {
		return nil, w.err
	}

	ctx, span := tracer.Start(ctx, "kv.TikvWrite.Get")
	defer span.End()

	b, err := w.txn.Get(ctx, key)
	if err != nil {
		log.Debug("[tikv].Get:", "key", string(key), "err", err)
		return b, err
	}
	log.Debug("[tikv].Get:", "key", string(key))
	return b, err
}

func (w *TikvWrite) Del(key []byte) error {
	if w.err != nil {
		return w.err
	}
	err := w.txn.Delete(key)
	if err != nil {
		w.err = err
	}
	return err
}

func (r *TikvWrite) Iter(ctx context.Context, start []byte, end []byte) iter.Seq2[KeyAndValue, error] {
	return func(yield func(KeyAndValue, error) bool) {

		_, span := tracer.Start(ctx, "kv.TikvWrite.Iter")
		defer span.End()

		it, err := r.txn.Iter(start, end)
		if err != nil {
			log.Debug("[tikv].Iter:", "start", string(start), "end", string(end), "err", err)
			yield(KeyAndValue{}, err)
			return
		}

		log.Debug("[tikv].Iter:", "start", string(start), "end", string(end))
		for it.Valid() {

			log.Debug("[tikv].Iter:", "start", string(start), "end", string(end), "at", string(it.Key()))
			if !yield(KeyAndValue{K: it.Key(), V: it.Value()}, nil) {
				return
			}

			err := it.Next()
			if err != nil {
				log.Debug("[tikv].Iter:", "start", string(start), "end", string(end), "err", err)
				if !yield(KeyAndValue{}, err) {
					return
				}
			}
		}
	}
}

func (r *TikvWrite) Close() {
	r.Rollback()
}

type TikvRead struct {
	txn *txnsnapshot.KVSnapshot
	err error
}

func (r *TikvRead) Get(ctx context.Context, key []byte) ([]byte, error) {
	if r.err != nil {
		return nil, r.err
	}

	ctx, span := tracer.Start(ctx, "kv.TikvRead.Get")
	defer span.End()

	b, err := r.txn.Get(ctx, key)
	if err != nil {
		log.Debug("[tikv].Get:", "key", string(key), "err", err)
		return b, err
	}
	log.Debug("[tikv].Get:", "key", string(key))
	return b, err
}

func (r *TikvRead) Close() {
}

func (r *TikvRead) SetKeyOnly(b bool) {
	r.txn.SetKeyOnly(b)
}

func (r *TikvRead) Iter(ctx context.Context, start []byte, end []byte) iter.Seq2[KeyAndValue, error] {
	return func(yield func(KeyAndValue, error) bool) {

		_, span := tracer.Start(ctx, "kv.TikvRead.Iter")
		defer span.End()

		it, err := r.txn.Iter(start, end)
		if err != nil {
			log.Debug("[tikv].Iter:", "start", string(start), "end", string(end), "err", err)
			yield(KeyAndValue{}, err)
			return
		}

		log.Debug("[tikv].Iter:", "start", string(start), "end", string(end))
		for it.Valid() {

			log.Debug("[tikv].Iter:", "start", string(start), "end", string(end), "at", string(it.Key()))
			if !yield(KeyAndValue{K: it.Key(), V: it.Value()}, nil) {
				return
			}

			err := it.Next()
			if err != nil {
				log.Debug("[tikv].Iter:", "start", string(start), "end", string(end), "err", err)
				if !yield(KeyAndValue{}, err) {
					return
				}
			}
		}
	}
}

func (r *TikvRead) BatchGet(ctx context.Context, keys [][]byte) (map[string][]byte, error) {
	if r.err != nil {
		return nil, r.err
	}

	ctx, span := tracer.Start(ctx, "kv.TikvRead.BatchGet")
	defer span.End()

	b, err := r.txn.BatchGet(ctx, keys)
	if err != nil {
		log.Debug("[tikv].BatchGet:", "keys", len(keys), "err", err)
		return b, err
	}
	log.Debug("[tikv].BatchGet:", "keys", len(keys))
	return b, err
}

func (r *TikvWrite) BatchGet(ctx context.Context, keys [][]byte) (map[string][]byte, error) {
	if r.err != nil {
		return nil, r.err
	}
	b, err := r.txn.BatchGet(ctx, keys)
	if err != nil {
		log.Debug("[tikv].BatchGet:", "keys", len(keys), "err", err)
		return b, err
	}
	log.Debug("[tikv].BatchGet:", "keys", len(keys))
	return b, err
}

func (t *Tikv) Close() {
	t.k.Close()
}

func (t *Tikv) Write() Write {
	txn, err := t.k.Begin()
	//txn.SetEnable1PC(true)
	return &TikvWrite{txn: txn, err: err}
}

func (t *Tikv) ExclusiveWrite(ctx context.Context, keys ...[]byte) (Write, error) {

	var waitMs = int64(100)

	// DO NOT use aggressive locking.
	// it's a deadlock trap
	// r.txn.StartAggressiveLocking()
	// instead do the retry loop in the client (us)

	txn, err := t.k.Begin()
	if err != nil {
		return nil, err
	}
	txn.SetPessimistic(true)

	retries := 0
	for {

		waitMs += 1
		retries += 1

		lkctx := kv.NewLockCtx(txn.StartTS(), waitMs, time.Now())

		err = txn.LockKeys(ctx, lkctx, keys...)
		if err == nil {
			break
		}

		// we got the lock but someone changed the key we're locking
		// get a new txn with the current start time
		if tikverr.IsErrWriteConflict(err) {
			txn.Rollback()
			txn, err = t.k.Begin()
			if err != nil {
				return nil, err
			}
			txn.SetPessimistic(true)
			continue
		}

		if !errors.Is(err, tikverr.ErrLockWaitTimeout) {
			return nil, err
		}

		select {
		case <-ctx.Done():
			if ctx.Err() == nil {
				return nil, err
			}
			return nil, ctx.Err()
		default:
			continue
		}
	}

	return &TikvWrite{txn: txn, statLockRetries: retries}, nil
}

func (t *Tikv) Read() Read {
	ts, err := t.k.CurrentTimestamp("global")
	if err != nil {
		return &TikvRead{nil, err}
	}

	txn := t.k.GetSnapshot(ts)
	return &TikvRead{txn, nil}
}

func (t *Tikv) Ping() error {
	_, err := t.k.CurrentTimestamp("global")
	return err
}

func NewTikv() (KV, error) {
	tikvep := os.Getenv("PD_ENDPOINT")
	if tikvep == "" {
		tikvep = "127.0.0.1:2379"
	}
	k, err := txnkv.NewClient([]string{tikvep})
	if err != nil {
		return nil, err
	}

	return &Tikv{k}, nil
}
