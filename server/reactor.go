package server

import (
	pb "apogy/proto"
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"log/slog"
	"strings"
)

func (s *server) reconcile(schema *pb.Document, doc *pb.Document) error {

	if schema.Val.Fields["reactors"] == nil {
		return nil
	}

	rol, ok := schema.Val.Fields["reactors"].Kind.(*structpb.Value_ListValue)
	if !ok {
		return nil
	}

	// FIXME this needs to mark somehow which reactor is currently activated and then move to the next

	var status = make(map[string]any)
	if doc.Status != nil {
		status = doc.Status.AsMap()
	}

	statusR, ok := status["reactor"].(map[string]any)
	if !ok {
		statusR = make(map[string]any)
	}

	for _, ro := range rol.ListValue.Values {
		reactorId, ok := ro.Kind.(*structpb.Value_StringValue)

		if !ok {
			continue
		}

		statusRR, ok := statusR[reactorId.StringValue].(map[string]any)
		if !ok {
			statusRR = make(map[string]any)
		}

		if v, ok := statusRR["done"].(uint64); ok {
			if v == *doc.Version {
				continue
			}
		}

		err := s.dura.Notify(reactorId.StringValue, doc.Model, doc.Id, *doc.Version)
		if err != nil {
			slog.Warn("failed to notify reactor "+reactorId.StringValue, "err", err)
		}

		statusRR["notified"] = *doc.Version
		statusR[reactorId.StringValue] = statusRR
	}

	status["reactor"] = statusR
	statusPBV, err := structpb.NewStruct(status)
	if err != nil {
		panic(err)
	}

	doc.Status = statusPBV

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

	err := s.dura.CreateReactor(object.Id)
	if err != nil {
		return err
	}

	return nil
}
