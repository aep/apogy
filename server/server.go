package server

import (
	bus "apogy/bus"
	kv "apogy/kv"
	pb "apogy/proto"
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"google.golang.org/grpc"
)

type server struct {
	pb.UnimplementedDocumentServiceServer
	pb.UnimplementedReactorServiceServer
	kv kv.KV
	bs bus.Bus
}

func newServer(kv kv.KV, bs bus.Bus) *server {
	return &server{
		kv: kv,
		bs: bs,
	}
}

func Main() {

	kv, err := kv.NewPebble()
	if err != nil {
		panic(err)
	}

	st, err := bus.NewSolo()
	if err != nil {
		panic(err)
	}

	s := newServer(kv, st)

	go func() {
		lis, err := net.Listen("tcp", ":5051")
		if err != nil {
			panic(fmt.Sprintf("failed to listen: %v", err))
		}

		grpcServer := grpc.NewServer(
			grpc.UnaryInterceptor(LoggingInterceptor),
		)
		pb.RegisterDocumentServiceServer(grpcServer, s)
		pb.RegisterReactorServiceServer(grpcServer, s)

		fmt.Println("Starting gRPC server on :5051")
		if err := grpcServer.Serve(lis); err != nil {
			panic(fmt.Sprintf("failed to serve: %v", err))
		}
	}()

	fmt.Println("Starting HTTP gateway server on :5052")
	if err := GatewayRun(); err != nil {
		panic(fmt.Sprintf("failed to serve gateway: %v", err))
	}
}

func LoggingInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	start := time.Now()
	h, err := handler(ctx, req)

	slog.Info("grpc", "method", info.FullMethod, "duration", time.Since(start), "err", err)
	return h, err
}
