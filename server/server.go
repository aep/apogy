package server

import (
	kv "apogy/kv"
	pb "apogy/proto"
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type server struct {
	pb.UnimplementedDocumentServiceServer
	kv kv.KV
}

func newServer(kv kv.KV) *server {
	return &server{
		kv: kv,
	}
}

func validateMeta(doc *pb.Document) error {

	if len(doc.Model) < 1 {
		return fmt.Errorf("validation error: /model must not be empty")
	}
	if len(doc.Model) > 64 {
		return fmt.Errorf("validation error: /model must be less than 64 bytes")
	}
	if len(doc.Id) < 1 {
		return fmt.Errorf("validation error: /id must not be empty")
	}
	if len(doc.Id) > 64 {
		return fmt.Errorf("validation error: /id must be less than 64 bytes")
	}

	for _, char := range doc.Model {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char == '.') ||
			(char == '-') ||
			(char >= '0' && char <= '9')) {
			return fmt.Errorf("validation error: /model has invalid character: %c", char)
		}
	}

	for _, char := range doc.Id {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char == '.') ||
			(char == '-') ||
			(char >= '0' && char <= '9')) {
			return fmt.Errorf("validation error: /id has invalid character: %c", char)
		}
	}

	return nil
}

func safeDBPath(model string, id string) ([]byte, error) {
	for _, ch := range model {
		if ch == 0xff {
			return nil, errors.New("invalid utf8 string")
		}
	}
	for _, ch := range id {
		if ch == 0xff {
			return nil, errors.New("invalid utf8 string")
		}
	}
	return []byte("o\xff" + model + "\xff" + id + "\xff"), nil
}

type Object map[string]interface{}

func (s *server) PutDocument(ctx context.Context, req *pb.PutDocumentRequest) (*pb.PutDocumentResponse, error) {

	err := validateMeta(req.Document)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	}

	if req.Document.Model == "Model" {
		return s.putSchema(ctx, req)
	}

	err = s.validateObjectSchema(ctx, req.Document)
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

			if original.Version != req.Document.Version {
				return nil, status.Errorf(codes.AlreadyExists, "version is out of date")
			}

		}
	} else {

		// even when user didnt request versioning we need to delete outdated index
		// use a separate reader because we don't care about conflicts
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
		}
	}

	if req.Document.Version == nil {
		var nuInt = uint64(0)
		req.Document.Version = &nuInt
	}
	*req.Document.Version += 1

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

func Main() {
	kv, err := kv.NewTikv()
	if err != nil {
		panic(err)
	}

	s := newServer(kv)

	// Start gRPC server in a goroutine
	go func() {
		lis, err := net.Listen("tcp", ":5051")
		if err != nil {
			panic(fmt.Sprintf("failed to listen: %v", err))
		}

		grpcServer := grpc.NewServer()
		pb.RegisterDocumentServiceServer(grpcServer, s)

		fmt.Println("Starting gRPC server on :5051")
		if err := grpcServer.Serve(lis); err != nil {
			panic(fmt.Sprintf("failed to serve: %v", err))
		}
	}()

	// Start the gateway
	fmt.Println("Starting HTTP gateway server on :5052")
	if err := GatewayRun(); err != nil {
		panic(fmt.Sprintf("failed to serve gateway: %v", err))
	}
}
