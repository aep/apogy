package kv

import (
	"context"
	"fmt"
	"iter"
	"log/slog"
	"os"

	pingcaplog "github.com/pingcap/log"

	"github.com/lmittmann/tint"
	"github.com/tikv/client-go/v2/txnkv"
	"github.com/tikv/client-go/v2/txnkv/txnsnapshot"

	"go.uber.org/zap"
)

func init() {
	l, p, _ := pingcaplog.InitLogger(&pingcaplog.Config{})

	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	config.OutputPaths = []string{"stderr"}
	config.ErrorOutputPaths = []string{"stderr"}
	l, _ = config.Build()

	pingcaplog.ReplaceGlobals(l, p)
}

var log = slog.New(tint.NewHandler(os.Stderr, nil))

type Tikv struct {
	k *txnkv.Client
}

type TikvWrite struct {
	txn      *txnkv.KVTxn
	err      error
	commited bool
}

func (w *TikvWrite) Commit(ctx context.Context) error {
	if w.err != nil {
		return w.err
	}
	if w.commited {
		return fmt.Errorf("already commited")
	}
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
	return w.txn.Delete(key)
}

func (r *TikvWrite) Iter(ctx context.Context, start []byte, end []byte) iter.Seq2[KeyAndValue, error] {

	return func(yield func(KeyAndValue, error) bool) {

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

func (t *Tikv) Close() {
	t.k.Close()
}

func (t *Tikv) Write() Write {
	txn, err := t.k.Begin()
	return &TikvWrite{txn, err, false}
}

func (t *Tikv) Read() Read {
	ts, err := t.k.CurrentTimestamp("global")
	if err != nil {
		return &TikvRead{nil, err}
	}

	txn := t.k.GetSnapshot(ts)
	return &TikvRead{txn, nil}
}

func NewTikv() (KV, error) {
	k, err := txnkv.NewClient([]string{"127.0.0.1:2379"})
	if err != nil {
		return nil, err
	}

	return &Tikv{k}, nil
}
