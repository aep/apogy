package server

import (
	pb "apogy/proto"
	"context"
	"fmt"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func GatewayRun() error {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Register gRPC-Gateway multiplexer
	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	err := pb.RegisterDocumentServiceHandlerFromEndpoint(
		ctx,
		mux,
		"localhost:5051", // Your gRPC server endpoint
		opts,
	)
	if err != nil {
		return fmt.Errorf("failed to register gateway: %v", err)
	}

	// Start HTTP server
	return http.ListenAndServe(":8080", mux)
}
