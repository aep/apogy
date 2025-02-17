package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"

	"github.com/aep/apogy/api/go"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		//TODO does this matter?
		return true
	},
}

// ReactorLoop handles the WebSocket connection for reactor operations
func (s *server) ReactorLoop(c echo.Context) error {
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()

	var cleanMe = make(map[string]func())
	defer func() {
		for _, f := range cleanMe {
			f()
		}
	}()

	// Channel for receiving messages from WebSocket
	recvch := make(chan openapi.ReactorIn)
	go s.handleWebSocketReceive(ws, recvch)

	// Wait for start message
	startMsg := <-recvch
	if startMsg.Start == nil {
		return fmt.Errorf("expected Start message, got %T", startMsg)
	}

	reactorIDB, err := safeDB(startMsg.Start.Id)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Define reactor trip function
	tripReactor := func(act *openapi.ReactorActivation) error {
		if act.Id == "" || act.Model == "" {
			return fmt.Errorf("invalid activation")
		}

		reactorKVPath, err := reactorKVPath(startMsg.Start.Id, act.Model, act.Id)
		if err != nil {
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		r := s.kv.Read()
		actBytes, err := r.Get(ctx, reactorKVPath)
		r.Close()

		var activation openapi.ReactorActivation
		if err == nil {
			err = json.Unmarshal(actBytes, &activation)
		}
		if err != nil || act.Id != activation.Id || act.Model != activation.Model {
			slog.Warn("BUG: activation loaded from kv didn't match caller",
				"err", err,
				"expectedId", act.Id, "loadedID", activation.Id,
				"expectedModel", act.Model, "loadedModel", activation.Model)

			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			w := s.kv.Write()
			w.Del(reactorKVPath)
			w.Commit(ctx)

			return nil
		}

		lockkey := fmt.Sprintf("r/%s/%s/%s", startMsg.Start.Id, activation.Model, activation.Id)

		lock, err := s.bs.Lock(context.Background(), lockkey, time.Second)
		if err != nil {
			return nil
		}

		cleanup := func() {
			lock.Unlock()
			delete(cleanMe, lockkey)
		}
		defer cleanup()
		cleanMe[lockkey] = cleanup

		// Send activation through WebSocket
		response := openapi.ReactorOut{
			Activation: &activation,
		}
		if err := ws.WriteJSON(response); err != nil {
			return err
		}

		err = func() error {
			for {
				select {
				case msg, ok := <-recvch:
					if !ok {
						return fmt.Errorf("websocket closed")
					}

					if msg.Working != nil {
						lock.KeepAlive()
						continue
					} else if msg.Done != nil {
						return nil
					}

				case <-lock.Done():
					return fmt.Errorf("keepalive deadline expired")
				}
			}
		}()
		if err != nil {
			return err
		}

		ctx, cancel = context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		w := s.kv.Write()

		b2, err := w.Get(ctx, reactorKVPath)
		if err != nil {
			slog.Warn(err.Error())
		}
		if bytes.Compare(b2, actBytes) != 0 {
			slog.Info("reactor trip was working on outdated version")
			w.Rollback()
			s.bs.Send("reactor-activate-"+startMsg.Start.Id, b2)
			return nil
		}

		w.Del(reactorKVPath)
		w.Commit(ctx)

		return nil
	}

	// Define replay function
	replay := func() error {
		itStart := []byte("r\xff")
		itStart = append(itStart, reactorIDB...)
		itStop := bytes.Clone(itStart)
		itStart = append(itStart, 0xff, 0x00)
		itStop = append(itStop, 0xff, 0xff)

		r := s.kv.Read()
		for kv, err := range r.Iter(context.Background(), itStart, itStop) {
			if err != nil {
				return err
			}

			idd := bytes.Split(kv.K, []byte{0xff})
			if len(idd) < 4 {
				continue
			}

			id := string(idd[len(idd)-2])
			model := string(idd[len(idd)-3])

			var activation openapi.ReactorActivation
			err := json.Unmarshal(kv.V, &activation)
			if err != nil || activation.Id != id || activation.Model != model {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				w := s.kv.Write()
				w.Del(kv.K)
				w.Commit(ctx)
				continue
			}

			err = tripReactor(&activation)
			if err != nil {
				slog.Warn("reactor trip", "err", err)
				return err
			}
		}
		r.Close()
		return nil
	}

	// Main event loop
	t := time.NewTicker(time.Second)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			if err := replay(); err != nil {
				return err
			}

		case msg, ok := <-recvch:
			if !ok {
				return nil
			}
			_ = msg

		case b2 := <-s.bs.Recv("reactor-activate-" + startMsg.Start.Id):
			if b2 == nil {
				slog.Warn("bus error", "b", b2)
				continue
			}
			var activation openapi.ReactorActivation
			if err := json.Unmarshal(b2, &activation); err != nil {
				slog.Warn("bus error", "err", err)
				continue
			}
			if err := tripReactor(&activation); err != nil {
				slog.Warn("reactor trip", "err", err)
				return err
			}
		}
	}
}

func (s *server) handleWebSocketReceive(ws *websocket.Conn, recvch chan openapi.ReactorIn) {
	defer close(recvch)
	for {
		var msg openapi.ReactorIn
		if err := ws.ReadJSON(&msg); err != nil {
			slog.Warn("WebSocket read error", "err", err)
			return
		}
		recvch <- msg
	}
}

func (s *server) reconcile(ctx context.Context, schema *openapi.Document, doc *openapi.Document) error {
	if schema.Val == nil || (*schema.Val)["reactors"] == nil {
		return nil
	}

	reactors, ok := (*schema.Val)["reactors"].([]interface{})
	if !ok {
		return nil
	}

	for _, r := range reactors {
		reactorID, ok := r.(string)
		if !ok {
			continue
		}

		activation := openapi.ReactorActivation{
			Id:      doc.Id,
			Model:   doc.Model,
			Version: *doc.Version,
		}

		actBytes, err := json.Marshal(&activation)
		if err != nil {
			return err
		}

		reactorKVPath, err := reactorKVPath(reactorID, doc.Model, doc.Id)
		if err != nil {
			continue
		}

		w := s.kv.Write()
		w.Put(reactorKVPath, actBytes)
		if err := w.Commit(ctx); err != nil {
			return err
		}

		s.bs.Send("reactor-activate-"+reactorID, actBytes)
		break
	}

	return nil
}

func (s *server) validateReactorSchema(ctx context.Context, object *openapi.Document) error {
	idparts := strings.FieldsFunc(object.Id, func(r rune) bool {
		return r == '.'
	})
	if len(idparts) < 3 {
		return fmt.Errorf("validation error (id): must be a domain, like com.example.Book")
	}
	return nil
}

func reactorKVPath(reactorID string, model string, docID string) ([]byte, error) {
	reactorIDB, err := safeDB(reactorID)
	if err != nil {
		return nil, err
	}

	modelB, err := safeDB(model)
	if err != nil {
		return nil, err
	}

	docIDB, err := safeDB(docID)
	if err != nil {
		return nil, err
	}

	rp := append([]byte("r\xff"), reactorIDB...)
	rp = append(rp, 0xff)
	rp = append(rp, modelB...)
	rp = append(rp, 0xff)
	rp = append(rp, docIDB...)
	rp = append(rp, 0xff)

	return rp, nil
}
