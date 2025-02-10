package stream

import (
	natsd "github.com/nats-io/nats-server/v2/server"
)

func NewEmbeddedNats() {

	opts := &natsd.Options{
		Host:      "localhost",
		Port:      4222,
		JetStream: true,         // Enable JetStream
		StoreDir:  "nats-store", // Directory for storing streams
	}

	natsd, err := natsd.NewServer(opts)
	if err != nil {
		panic(err)
	}

	go natsd.Start()

}
