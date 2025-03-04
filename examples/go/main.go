package main

import (
	"context"
	"fmt"
	"github.com/aep/apogy/examples/go/apogy"
)

func main() {
	client, err := apogy.NewClient("http://localhost:27666")
	if err != nil {
		panic(err)
	}

	doc, err := client.Book.Get(context.Background(), "b7c89d01-2y34-5z67-a890-b123c4567d89")
	if err != nil {
		panic(err)
	}

	fmt.Println("changing book titled", doc.Val.Name)

	doc.Val.Author = "Me lol"
	err = client.Book.Put(context.Background(), doc)
	if err != nil {
		panic(err)
	}

	fmt.Println("listing all books")

	for doc, err := range client.Book.Query(context.Background(), "com.example.Book") {
		if err != nil {
			panic(err)
		}
		fmt.Println(doc.Id)
	}
}
