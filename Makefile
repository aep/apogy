all: apogy


api: .PHONY
	make -C api

apogy: api .PHONY
	go build



test:
	go test github.com/aep/apogy/server/...


.PHONY:
