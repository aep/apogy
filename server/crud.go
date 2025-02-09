package server

import (
	pb "apogy/proto"
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *server) PutDocument(ctx context.Context, req *pb.PutDocumentRequest) (*pb.PutDocumentResponse, error) {

	err := validateMeta(req.Document)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	}

	if req.Document.Model == "Model" {
		return s.putSchema(ctx, req)
	} else if req.Document.Model == "Reactor" {
		return s.putReactor(ctx, req)
	}

	schema, err := s.validateObjectSchema(ctx, req.Document)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "validation error: %s", err)
	}

	w2 := s.kv.Write()
	defer w2.Close()

	path, err := safeDBPath(req.Document.Model, req.Document.Id)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	now := time.Now()
	req.Document.History = &pb.History{
		Created: timestamppb.New(now),
		Updated: timestamppb.New(now),
	}

	if req.Document.Version != nil {

		bytes, err := w2.Get(ctx, []byte(path))
		if err != nil {
			if !strings.Contains(err.Error(), "not exist") {
				return nil, fmt.Errorf("tikv error %v", err)
			}
		} else {
			var original pb.Document
			err = proto.Unmarshal(bytes, &original)
			if err != nil {
				return nil, fmt.Errorf("unmarshal error: %v", err)
			}

			deleteIndex(w2, &original)

			if original.History != nil {
				req.Document.History.Created = original.History.Created
			}

			if reflect.DeepEqual(original.Val, req.Document.Val) {
				return &pb.PutDocumentResponse{
					Path: req.Document.Model + "/" + req.Document.Id,
				}, nil
			}

			if original.Version != nil && req.Document.Version != nil {
				if *original.Version != *req.Document.Version {
					return nil, status.Errorf(codes.AlreadyExists, "version is out of date")
				}
			}

		}
	} else {

		// user doesnt want versioning. use a separate reader so we can ignore conflicts
		r := s.kv.Read()
		bytes, err := r.Get(ctx, []byte(path))
		r.Close()
		if err != nil {
			if !strings.Contains(err.Error(), "not exist") {
				return nil, fmt.Errorf("tikv error %v", err)
			}
		} else {
			var original pb.Document
			err = proto.Unmarshal(bytes, &original)
			if err != nil {
				return nil, fmt.Errorf("unmarshal error: %v", err)
			}
			deleteIndex(w2, &original)

			if original.History != nil {
				req.Document.History.Created = original.History.Created
			}

			req.Document.Version = original.Version

		}
	}

	if req.Document.Version == nil {
		var nuInt = uint64(0)
		req.Document.Version = &nuInt
	}
	*req.Document.Version += 1

	err = s.reconcile(schema, req.Document)
	if err != nil {
		return nil, err
	}

	bytes, err := proto.Marshal(req.Document)
	if err != nil {
		return nil, fmt.Errorf("marshal error: %v", err)
	}

	w2.Put([]byte(path), bytes)

	err = createIndex(w2, req.Document)
	if err != nil {
		return nil, err
	}

	err = w2.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("tikv error: %v", err)
	}

	return &pb.PutDocumentResponse{
		Path: req.Document.Model + "/" + req.Document.Id,
	}, nil
}

func (s *server) GetDocument(ctx context.Context, req *pb.GetDocumentRequest) (*pb.Document, error) {

	path, err := safeDBPath(req.Model, req.Id)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	r := s.kv.Read()
	defer r.Close()

	bytes, err := r.Get(ctx, []byte(path))
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}
	if bytes == nil {
		return nil, status.Error(codes.NotFound, "not found")
	}

	var doc = new(pb.Document)
	err = proto.Unmarshal(bytes, doc)
	if err != nil {
		return nil, status.Error(codes.Internal, "unmarshal error")
	}

	return doc, nil
}
