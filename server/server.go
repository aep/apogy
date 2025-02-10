package server

import (
	dura "apogy/dura"
	kv "apogy/kv"
	pb "apogy/proto"
	"fmt"
	"net"

	"google.golang.org/grpc"
)

type server struct {
	pb.UnimplementedDocumentServiceServer
	kv   kv.KV
	dura *dura.Nats
}

func newServer(kv kv.KV, dura *dura.Nats) *server {
	return &server{
		kv:   kv,
		dura: dura,
	}
}

func Main() {
	kv, err := kv.NewPebble()
	if err != nil {
		panic(err)
	}

	dura, err := dura.ConnectNats()
	if err != nil {
		panic(err)
	}

	s := newServer(kv, dura)

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
