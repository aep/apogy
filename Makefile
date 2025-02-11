all: apogy


api: .PHONY
	make -C api

apogy: api .PHONY
	go build


.PHONY:
