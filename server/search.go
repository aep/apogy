package server

import (
	"apogy/proto"
	pb "apogy/proto"
	"bytes"
	"context"
	"fmt"
	"iter"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	kv "apogy/kv"
)

func (s *server) find(ctx context.Context, r kv.Read, model string, byID *string,
	cursor *string, filter *proto.Filter) iter.Seq2[string, error] {

	var start = []byte{'f', 0xff}
	start = append(start, []byte(model)...)
	start = append(start, 0xff)
	start = append(start, []byte(filter.Key)...)

	switch v := filter.Condition.(type) {
	case *proto.Filter_Equal:
		switch v := v.Equal.Kind.(type) {
		case *structpb.Value_StringValue:
			start = append(start, 0xff)
			start = append(start, v.StringValue...)
			start = append(start, 0xff)

		}

		if byID != nil {
			start = append(start, []byte(*byID)...)
		}
	default:
		start = append(start, 0x00)
	}

	end := bytes.Clone(start)
	end[len(end)-2] = end[len(end)-2] + 1

	fmt.Println(escapeNonPrintable(start))
	fmt.Println(escapeNonPrintable(end))

	if cursor != nil {
		// FIXME check if cursor is above start
		// FIXME change start to cursor
	}

	return func(yield func(string, error) bool) {

		var seen = make(map[string]bool)

		for kv, err := range r.Iter(ctx, start, end) {

			if err != nil {
				yield("", err)
				return
			}

			kk := bytes.Split(kv.K, []byte{0xff})
			if len(kk) < 3 {
				continue
			}
			id := string(kk[len(kk)-1])

			if byID != nil {
				if *byID != id {
					continue
				}
			}

			if seen[id] {
				continue
			}
			seen[id] = true

			if !yield(id, nil) {
				return
			}
		}
	}
}

func (s *server) SearchDocuments(ctx context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {

	for _, ch := range req.Model {
		if ch == 0xff {
			return nil, status.Errorf(codes.InvalidArgument, "invalid utf8 string")
		}
	}
	for _, ch := range req.Cursor {
		if ch == 0xff {
			return nil, status.Errorf(codes.InvalidArgument, "invalid utf8 string")
		}
	}
	for _, f := range req.Filters {
		for _, ch := range f.Key {
			if ch == 0xff {
				return nil, status.Errorf(codes.InvalidArgument, "invalid utf8 string")
			}
		}

		switch v := f.Condition.(type) {
		case *proto.Filter_Equal:
			switch v := v.Equal.Kind.(type) {
			case *structpb.Value_StringValue:
				for _, ch := range v.StringValue {
					if ch == 0xff {
						return nil, status.Errorf(codes.InvalidArgument, "invalid utf8 string")
					}
				}
			}
		case *proto.Filter_Less:
			switch v := v.Less.Kind.(type) {
			case *structpb.Value_StringValue:
				for _, ch := range v.StringValue {
					if ch == 0xff {
						return nil, status.Errorf(codes.InvalidArgument, "invalid utf8 string")
					}
				}
			}
		case *proto.Filter_Greater:
			switch v := v.Greater.Kind.(type) {
			case *structpb.Value_StringValue:
				for _, ch := range v.StringValue {
					if ch == 0xff {
						return nil, status.Errorf(codes.InvalidArgument, "invalid utf8 string")
					}
				}
			}
		}
	}

	if len(req.Filters) == 0 || req.Model == "" {
		return nil, status.Errorf(codes.InvalidArgument, "invalid query")
	}

	r := s.kv.Read()
	if r, ok := r.(*kv.TikvRead); ok {
		r.SetKeyOnly(true)
	}
	defer r.Close()

	// TODO cursor

	var rsp = new(proto.SearchResponse)

	for k, err := range s.find(ctx, r, req.Model, nil, nil, req.Filters[0]) {
		if err != nil {
			return nil, err
		}

		allMatch := true
		for _, fine := range req.Filters[1:] {

			thisMatch := false
			for k2, err := range s.find(ctx, r, req.Model, &k, nil, fine) {
				if err != nil {
					return nil, err
				}
				if k == k2 {
					thisMatch = true
					break
				}
			}
			if !thisMatch {
				allMatch = false
				break
			}
		}

		if !allMatch {
			continue
		}
		rsp.Ids = append(rsp.Ids, k)
	}

	return rsp, nil
}

func escapeNonPrintable(b []byte) string {
	var result strings.Builder
	for _, c := range b {
		if c >= 32 && c <= 126 {
			result.WriteByte(c)
		} else {
			result.WriteString(fmt.Sprintf("\\x%02x", c))
		}
	}
	return result.String()
}
