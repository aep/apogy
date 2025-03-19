package kv

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

func TestKVDoesPreventOutdatedWrites(t *testing.T) {

	k, err := NewTikv()
	require.NoError(t, err)
	defer k.Close()

	w1 := k.Write()
	w2 := k.Write()
	w3 := k.Write()

	w1.Put([]byte("alice"), []byte("1"))
	w2.Put([]byte("alice"), []byte("2"))
	w3.Put([]byte("bob"), []byte("3"))

	err = w1.Commit(context.Background())
	require.NoError(t, err)

	err = w2.Commit(context.Background())
	require.Error(t, err, "w2 must fail because it is older than the last write")

	err = w3.Commit(context.Background())
	require.NoError(t, err, "w3 must succeed because it is writing an unrelated key")

	w4 := k.Write()
	w4.Put([]byte("alice"), []byte("4"))
	err = w4.Commit(context.Background())
	require.NoError(t, err, "w4 must succeed because it is fresh")
}

// this behaviour actually sucks because it means Delete in apogy must be locked
// this test is just here in case they change their mind about it
func TestKVDoesPreventOutdatedDoubleDelete(t *testing.T) {

	k, err := NewTikv()
	require.NoError(t, err)
	defer k.Close()

	var ctx = t.Context()
	var key = []byte("TestKVDoesNotPreventOutdatedDoubleDelete")

	prep := k.Write()
	prep.Put(key, []byte("1"))
	err = prep.Commit(ctx)
	require.NoError(t, err)

	w1 := k.Write()
	w2 := k.Write()

	w1.Del(key)
	w2.Del(key)

	err = w1.Commit(context.Background())
	require.NoError(t, err)

	err = w2.Commit(context.Background())
	require.Error(t, err)
}

func TestKVDeleteAndPutMustConflict1(t *testing.T) {

	k, err := NewTikv()
	require.NoError(t, err)
	defer k.Close()

	var ctx = t.Context()
	var key = []byte("TestKVDoesNotPreventOutdatedDoubleDelete")

	prep := k.Write()
	prep.Put(key, []byte("1"))
	err = prep.Commit(ctx)
	require.NoError(t, err)

	w1 := k.Write()
	w2 := k.Write()

	w1.Put(key, []byte("2"))
	w2.Del(key)

	err = w1.Commit(context.Background())
	require.NoError(t, err)

	err = w2.Commit(context.Background())
	require.Error(t, err)
}

func TestKVDeleteAndPutMustConflict2(t *testing.T) {

	k, err := NewTikv()
	require.NoError(t, err)
	defer k.Close()

	var ctx = t.Context()
	var key = []byte("TestKVDoesNotPreventOutdatedDoubleDelete")

	prep := k.Write()
	prep.Put(key, []byte("1"))
	err = prep.Commit(ctx)
	require.NoError(t, err)

	w1 := k.Write()
	w2 := k.Write()

	w1.Del(key)
	w2.Put(key, []byte("2"))

	err = w1.Commit(context.Background())
	require.NoError(t, err)

	err = w2.Commit(context.Background())
	require.Error(t, err)
}

func TestKVDoesNotPreventDup(t *testing.T) {
	k, err := NewTikv()
	require.NoError(t, err)
	defer k.Close()

	w1 := k.Write()
	w1.Put([]byte("alice"), []byte("1"))
	w1.Put([]byte("alice"), []byte("2"))

	err = w1.Commit(context.Background())
	require.NoError(t, err, "kv must allow setting the same key twice within a tx")
}

func TestKVLockReordering(t *testing.T) {
	k, err := NewTikv()
	require.NoError(t, err)
	defer k.Close()

	var ctx = t.Context()
	var key = []byte("reordertest")

	w1, err := k.ExclusiveWrite(ctx, key)
	require.NoError(t, err)
	defer w1.Rollback()

	var w1commited = atomic.NewBool(false)
	go func() {
		time.Sleep(time.Millisecond * 101)

		err = w1.Put(key, []byte("1"))
		require.NoError(t, err)

		err = w1.Commit(ctx)
		w1commited.Store(true)
		require.NoError(t, err, "first commit must work")
	}()

	w2, err := k.ExclusiveWrite(ctx, key)
	require.NoError(t, err)
	defer w2.Rollback()

	if !w1commited.Load() {
		panic("not reordered")
	}

	current, err := w2.Get(ctx, key)
	require.NoError(t, err)
	require.Equal(t, "1", string(current))

	require.NoError(t, err)
	err = w2.Put(key, []byte("2"))
	require.NoError(t, err)

	err = w2.Commit(ctx)
	require.NoError(t, err, "second commit must work")

	actual, err := k.Read().Get(ctx, key)
	require.NoError(t, err)
	require.Equal(t, "2", string(actual))

}
