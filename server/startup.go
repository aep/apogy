package server

import (
	"context"
	"log/slog"
)

func (s *server) startup() {

	var ctx = context.Background()

	dbr := s.kv.Read()
	defer dbr.Close()

	docs, err := s.find(ctx, dbr, "Reactor", "", nil, 100000, nil)
	if err != nil {
		panic(err)
	}

	docs.documents, err = s.resolveFullDocs(ctx, dbr, docs.documents)
	if err != nil {
		panic(err)
	}

	for _, doc := range docs.documents {
		_, err := s.ro.Validate(ctx, nil, &doc)
		if err != nil {
			slog.Error("startup error", "err", err)
		}
	}

	docs, err = s.find(ctx, dbr, "Model", "", nil, 100000, nil)
	if err != nil {
		panic(err)
	}

	docs.documents, err = s.resolveFullDocs(ctx, dbr, docs.documents)
	if err != nil {
		panic(err)
	}

	for _, doc := range docs.documents {
		_, err := s.ro.Validate(ctx, nil, &doc)
		if err != nil {
			slog.Error("startup error", "err", err)
		}
	}
}
