package server

import (
	pb "apogy/proto"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

func (s *server) reconcile(ctx context.Context, schema *pb.Document, doc *pb.Document) error {

	if schema.Val.Fields["reactors"] == nil {
		return nil
	}

	rol, ok := schema.Val.Fields["reactors"].Kind.(*structpb.Value_ListValue)
	if !ok {
		return nil
	}

	for _, ro := range rol.ListValue.Values {
		reactorId, ok := ro.Kind.(*structpb.Value_StringValue)
		if !ok {
			continue
		}

		activation := pb.ReactorActivation{
			Id:      doc.Id,
			Model:   doc.Model,
			Version: *doc.Version,
		}

		actBytes, err := proto.Marshal(&activation)
		if err != nil {
			return err
		}

		reactorKVPath, err := reactorKVPath(reactorId.StringValue, doc.Model, doc.Id)
		if err != nil {
			continue
		}

		w := s.kv.Write()
		w.Put(reactorKVPath, actBytes)
		err = w.Commit(ctx)
		if err != nil {
			return err
		}

		s.bs.Send("reactor-notification", actBytes)

		//FIXME: this needs go to the next reactor if the previous one is completed

		break
	}

	return nil
}

func (s *server) validateReactorSchema(ctx context.Context, object *pb.Document) error {
	idparts := strings.FieldsFunc(object.Id, func(r rune) bool {
		return r == '.'
	})
	if len(idparts) < 3 {
		return status.Errorf(codes.InvalidArgument, "validation error (id): must be a domain , like com.example.Book")
	}
	return nil
}

func (s *server) ensureReactor(ctx context.Context, object *pb.Document) error {
	return nil
}

func (s *server) ReactorLoop(bidi grpc.BidiStreamingServer[pb.ReactorIn, pb.ReactorOut]) error {

	var cleanMe = make(map[string]func())
	defer func() {
		for _, f := range cleanMe {
			f()
		}
	}()

	recvch := make(chan *pb.ReactorIn)
	go func() {
		defer close(recvch)
		for {
			m, err := bidi.Recv()
			if err != nil {
				slog.Warn("ReactorLoop.bidi.Recv()", "err", err)
				return
			}
			recvch <- m
		}
	}()

	// await start message

	m := <-recvch
	start, ok := m.Kind.(*pb.ReactorIn_Start)
	if !ok {
		return fmt.Errorf("expected Start message")
	}

	reactorIDB, err := safeDB(start.Start.Id)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	var tripReactor = func(act *pb.ReactorActivation) error {

		reactorKVPath, err := reactorKVPath(start.Start.Id, act.Model, act.Id)
		if err != nil {
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		r := s.kv.Read()
		actBytes, err := r.Get(ctx, reactorKVPath)
		r.Close()

		var activation = new(pb.ReactorActivation)
		if err == nil {
			err = proto.Unmarshal(actBytes, activation)
		}
		if err != nil || act.Id != activation.Id || act.Model != activation.Model {

			slog.Warn("BUG: activation loaded from kv didnt match caller",
				"err", err,
				"expectedId", act.Id, "loadedID", activation.Id)

			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			w := s.kv.Write()
			w.Del(reactorKVPath)
			w.Commit(ctx)

			return nil
		}

		lockkey := "r/" + start.Start.Id + "/" + activation.Model + "/" + activation.Id

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

		bidi.Send(&pb.ReactorOut{
			Kind: &pb.ReactorOut_Activation{
				Activation: activation,
			},
		})

		err = func() error {
			for {
				select {
				case m, ok := <-recvch:
					if !ok || m == nil || m.Kind == nil {
						return io.EOF
					}
					switch m.Kind.(type) {
					case *pb.ReactorIn_Working:
						lock.KeepAlive()
					case *pb.ReactorIn_Done:
						return nil
					}
				case <-lock.Done():
					return status.Error(codes.DeadlineExceeded, "keepalive deadline expired")
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
			s.bs.Send("reactor-notification", b2)
			return nil
		}

		w.Del(reactorKVPath)
		w.Commit(ctx)

		return nil

	}

	var replay = func() error {

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

			var activation = new(pb.ReactorActivation)
			err := proto.Unmarshal(kv.V, activation)
			if err != nil || id != activation.Id || model != activation.Model {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				w := s.kv.Write()
				w.Del(kv.K)
				w.Commit(ctx)
			}

			err = tripReactor(activation)
			if err != nil {
				slog.Warn("reactor trip", "err", err)
				return err
			}

		}
		r.Close()
		return nil
	}

	t := time.NewTicker(time.Second)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			err = replay()
			if err != nil {
				return err
			}
		case _, ok := <-recvch:
			if !ok {
				return nil
			}
		case b2 := <-s.bs.Recv("reactor-notification"):
			if b2 == nil {
				slog.Warn("bus error", "b", b2)
				continue
			}
			var activation = new(pb.ReactorActivation)
			err = proto.Unmarshal(b2, activation)
			if err != nil {
				slog.Warn("bus error", "err", err)
				continue
			}
			err = tripReactor(activation)
			if err != nil {
				slog.Warn("reactor trip", "err", err)
				return err
			}
		}

	}
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
