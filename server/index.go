package server

import (
	"apogy/api"
	"apogy/kv"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
)

func deleteIndex(w kv.Write, object api.Object) error {
	pathPrefix := []byte(fmt.Sprintf("f\xff%s\xffval", object.Model))
	pathPostfix := []byte(fmt.Sprintf("%s", object.Id))
	return writeIndexI(w, pathPrefix, pathPostfix, object.Val, true)
}

func createIndex(w kv.Write, object api.Object) error {
	pathPrefix := []byte(fmt.Sprintf("f\xff%s\xffval", object.Model))
	pathPostfix := []byte(fmt.Sprintf("%s", object.Id))
	return writeIndexI(w, pathPrefix, pathPostfix, object.Val, false)
}

func writeIndexI(w kv.Write, pathPrefix []byte, pathPostfix []byte, obj any, delete bool) error {
	switch v := obj.(type) {

	case []interface{}:
		for _, v := range v {
			err := writeIndexI(w, pathPrefix, pathPostfix, v, delete)
			if err != nil {
				return err
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
				pathPrefix2 := append(pathPrefix, '.')
				pathPrefix2 = append(pathPrefix2, kbin...)
				err := writeIndexI(w, pathPrefix2, pathPostfix, v, delete)
				if err != nil {
					return err
				}
			}
		}
	case string:

		vbin := []byte(v)
		safe := true
		for _, ch := range vbin {
			if ch == 0xff {
				safe = false
				break
			}
		}
		if safe && len(vbin) < 128 {
			p := pathPrefix
			p = append(p, 0xff)
			p = append(p, vbin...)
			p = append(p, 0xff)
			p = append(p, pathPostfix...)

			if delete {
				w.Del(p)
			} else {
				w.Put(p, []byte{0})
			}
		}

	case float64:

		bits := math.Float64bits(v)
		bytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(bytes, bits)

		p := pathPrefix
		p = append(p, 0xff)
		p = append(p, bytes...)
		p = append(p, 0xff)
		p = append(p, pathPostfix...)
		if delete {
			w.Del(p)
		} else {
			w.Put(p, []byte{0})
		}

	default:
		slog.Warn(fmt.Sprintf("writeIndexI doesnt implement index for type %T ", obj))

	}

	return nil
}
