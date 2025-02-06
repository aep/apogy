package kv

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
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
