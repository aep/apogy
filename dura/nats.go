package dura

import (
	"encoding/json"
	"fmt"
	"github.com/nats-io/nats.go"
	"strings"
)

type Nats struct {
	nc *nats.Conn
	js nats.JetStreamContext
}

func ConnectNats() (*Nats, error) {

	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		panic(fmt.Sprintf("failed to create jetstream context: %v", err))
	}

	return &Nats{
		js: js,
		nc: nc,
	}, nil

}

func (n *Nats) Close() {
	n.nc.Close()
}

func (n *Nats) CreateReactor(id string) error {

	_, err := n.js.AddStream(&nats.StreamConfig{
		Name: "durabase-reactor-" + strings.ReplaceAll(id, ".", "_") + "-inbox",
		Subjects: []string{
			"durabase-reactor." + id + ".inbox.>",
			"durabase-reactor." + id + ".cache.>",
		},

		Replicas: 1, //TODO
		//Storage:  nats.MemoryStorage,
		Storage: nats.FileStorage,

		MaxMsgsPerSubject: 1,
		Discard:           nats.DiscardNew,
	})
	if err != nil {
		return fmt.Errorf("Error creating jetstream [needs a nats-server with -js] : %w", err)
	}

	return nil
}

func (n *Nats) DeleteReactor(id string) error {
	err := n.js.DeleteStream("durabase-reactor-" + id + "-inbox")
	if err != nil {
		return fmt.Errorf("Error deleting jetstream: %w", err)
	}
	return nil
}

type reactorWorkMessage struct {
	Model   string
	Id      string
	Version uint64
}

func (n *Nats) Notify(reactor string, model string, id string, version uint64) error {

	js, err := json.Marshal(&reactorWorkMessage{
		Model:   model,
		Id:      id,
		Version: version,
	})
	if err != nil {
		return err
	}

	_, err = n.js.Publish("durabase-reactor."+reactor+".inbox."+model+"."+id, js)
	if err != nil {
		return err
	}

	return nil
}
