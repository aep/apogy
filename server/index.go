package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"

	"encoding/json"
	openapi "github.com/aep/apogy/api/go"
	"github.com/aep/apogy/kv"
)

func (s *server) deleteIndex(ctx context.Context, w kv.Write, model *Model, object *openapi.Document) error {
	return s.writeIndexI(ctx, w, model, []byte(object.Id), "val", object.Val, true)
}

func (s *server) createIndex(ctx context.Context, w kv.Write, model *Model, object *openapi.Document) error {
	return s.writeIndexI(ctx, w, model, []byte(object.Id), "val", object.Val, false)
}

func (s *server) writeIndexI(ctx context.Context, w kv.Write, model *Model, objectId []byte, path string, obj any, delete bool) error {

	switch v := obj.(type) {

	case []interface{}:
		for _, v := range v {
			err := s.writeIndexI(ctx, w, model, objectId, path, v, delete)
			if err != nil {
				return err
			}
		}
	case *map[string]interface{}:
		if v == nil {
			return nil
		}
		for k, v := range *v {

			// make extra sure there is no 0xff anywhere in the data
			// it's not valid utf8 so this should not happen
			// if i dont check it, i'll probably make a mistake later that will allow a filter bypass

			kbin := []byte(k)
			safe := true
			for _, ch := range kbin {
				if ch == 0xff {
					safe = false
					break
				}
			}

			if safe {
				path2 := path + "." + k
				err := s.writeIndexI(ctx, w, model, objectId, path2, v, delete)
				if err != nil {
					return err
				}
			}
		}
	case map[string]interface{}:
		for k, v := range v {

			// make extra sure there is no 0xff anywhere in the data
			// it's not valid utf8 so this should not happen
			// if i dont check it, i'll probably make a mistake later that will allow a filter bypass

			kbin := []byte(k)
			safe := true
			for _, ch := range kbin {
				if ch == 0xff {
					safe = false
					break
				}
			}

			if safe {
				path2 := path + "." + k
				err := s.writeIndexI(ctx, w, model, objectId, path2, v, delete)
				if err != nil {
					return err
				}
			}
		}
	case string:

		unique := model.Index[path] == "unique"

		if len(v) > 128 {
			if unique {
				return fmt.Errorf("value of %s too large for unique index", path)
			}
			return nil
		}

		vbin := []byte(v)
		safe := true
		for _, ch := range vbin {
			if ch == 0xff {
				safe = false
				break
			}
		}

		if !safe {
			if unique {
				return fmt.Errorf("value of %s too large for unique index", path)
			}
			return nil
		}

		p := []byte("f\xff")
		p = append(p, []byte(model.Id)...)
		p = append(p, 0xff)
		p = append(p, []byte(path)...)
		p = append(p, 0xff)
		p = append(p, vbin...)
		p = append(p, 0xff)

		if unique {

			p := append(p, 0xff)
			if delete {
				w.Del(p)
			} else {
				ev, err := w.Get(ctx, p)
				if err == nil {
					evv := bytes.Split(ev, []byte{0xff})
					return fmt.Errorf("unique index in key %s is already set by document id %s", path, string(evv[0]))
				}
				w.Put(p, append(objectId, 0xff))
			}
		}

		if safe && len(vbin) < 128 {
			p = append(p, objectId...)
			p = append(p, 0xff)

			if delete {
				w.Del(p)
			} else {
				w.Put(p, append(objectId, 0xff))
			}
		}

	case json.Number:

		vbin := make([]byte, 8)
		if i64, err := v.Int64(); err == nil {
			binary.LittleEndian.PutUint64(vbin, uint64(i64))
		} else if f64, err := v.Float64(); err == nil {
			bits := math.Float64bits(f64)
			binary.LittleEndian.PutUint64(vbin, bits)
		} else {
			return nil
		}

		unique := model.Index[path] == "unique"

		p := []byte("f\xff")
		p = append(p, []byte(model.Id)...)
		p = append(p, 0xff)
		p = append(p, []byte(path)...)
		p = append(p, 0xff)
		p = append(p, vbin...)
		p = append(p, 0xff)

		if unique {
			p := append(p, 0xff)
			ev, err := w.Get(ctx, p)
			if err == nil {
				evv := bytes.Split(ev, []byte{0xff})
				return fmt.Errorf("unique index in key %s is already set by document id %s", path, string(evv[0]))
			}
		}

		p = append(p, objectId...)
		p = append(p, 0xff)

		if delete {
			w.Del(p)
		} else {
			w.Put(p, append(objectId, 0xff))
		}

	default:
		slog.Warn(fmt.Sprintf("writeIndexI doesnt implement index for type %T ", obj))

	}

	return nil
}
