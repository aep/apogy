all: apogy


api: .PHONY
	make -C api

apogy: api .PHONY
	go build



test:
	go test -count=1  -v github.com/aep/apogy/server/...


.PHONY:
