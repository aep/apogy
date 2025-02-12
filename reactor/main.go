package reactor

import (
	"apogy/api/go"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
)

type Reactor struct {
	lock    sync.RWMutex
	running map[string]*running
}

func New() *Reactor {
	return &Reactor{
		running: make(map[string]*running),
	}
}

// should be called by loading all the docs from db, but also when creating one at runtime
func (self *Reactor) Start(doc *openapi.Document) error {

	if doc.Model != "Reactor" {
		return fmt.Errorf("incorrect model passed to Reactor.Start")
	}
	if doc.Val == nil {
		return nil
	}

	is, ok := (*doc.Val)["validator"].(bool)
	if !ok || !is {
		return nil
	}
	codeStr, ok := (*doc.Val)["wasm"].(string)
	if !ok {
		return nil
	}
	code, err := base64.StdEncoding.DecodeString(codeStr)
	if err != nil {
		return err
	}

	r, err := startAssemblyScript(code)
	if err != nil {
		return err
	}

	self.lock.Lock()
	defer self.lock.Unlock()

	old := self.running[doc.Id]
	if old != nil {
		old.Close()
	}

	slog.Info("started validator", "reactorID", doc.Id)
	self.running[doc.Id] = r

	return nil
}

func (self *Reactor) Validate(ctx context.Context, reactorID string, old *openapi.Document, nuw *openapi.Document) error {

	self.lock.RLock()
	defer self.lock.RUnlock()

	rw := self.running[reactorID]
	if rw == nil {
		slog.Warn("called validator that is not running", "reactorID", reactorID)
		return nil
	}

	var oldJson []byte
	if old != nil {
		var err error
		oldJson, err = json.Marshal(old)
		if err != nil {
			return err
		}
	}

	nuwJson, err := json.Marshal(nuw)
	if err != nil {
		return err
	}

	return rw.validate(ctx, string(oldJson), string(nuwJson))

}

func (wr *running) Close() {
	wr.wr.Close(wr.ctx)
	wr.cancel()
}
