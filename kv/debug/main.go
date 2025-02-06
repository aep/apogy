package main

import (
	"fmt"
	"github.com/tikv/client-go/v2/txnkv"
)

func main() {
	k, err := txnkv.NewClient([]string{"127.0.0.1:2379"})
	if err != nil {
		panic(err)
	}
	defer k.Close()

	ts, err := k.CurrentTimestamp("global")
	if err != nil {
		panic(err)
	}

	txn := k.GetSnapshot(ts)
	txn.SetKeyOnly(true)

	it, err := txn.Iter(nil, nil)
	if err != nil {
		panic(err)
	}

	for it.Valid() {

		fmt.Println(string(it.Key()))

		err := it.Next()
		if err != nil {
			panic(err)
		}
	}
}
