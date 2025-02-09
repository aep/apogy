
ALL: proto build




build: proto .PHONY
	go build


PROTO_FILES=$(wildcard proto/*.proto)
PROTO_GO_FILES=$(PROTO_FILES:.proto=.pb.go)
PROTO_GRPC_FILES=$(PROTO_FILES:.proto=_grpc.pb.go)
proto: $(PROTO_GO_FILES) $(PROTO_GRPC_FILES)

%.pb.go %_grpc.pb.go: %.proto
	protoc \
		-I proto \
		-I proto/googleapis \
		--go_out=proto --go_opt=paths=source_relative \
		--go-grpc_out=proto --go-grpc_opt=paths=source_relative \
		--grpc-gateway_out=proto --grpc-gateway_opt paths=source_relative \
		$<


deps:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.32.0
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.3.0




.PHONY:
