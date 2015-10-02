mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
top := $(dir $(mkfile_path))

all: image

image: docker.img

docker.img: bp2/hooks/onos-hook bp2/hooks/onos-wrapper bp2/hooks/onos-service Dockerfile
	docker build --tag=ciena/onos:1.3  .
	touch docker.img

bp2/hooks/onos-wrapper:
	GOPATH=$(top)/vendor:$(top) CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -o bp2/hooks/onos-wrapper wrapper

bp2/hooks/onos-hook:
	GOPATH=$(top)/vendor:$(top) CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -o bp2/hooks/onos-hook hook

clean:
	rm -rf docker.img *~ bp2/hooks/onos-hook bp2/hooks/onos-wrapper bin pkg

bp2/hooks/onos-wrapper: \
	src/wrapper/wrapper.go \
	src/onos/onos.go \
	src/onos/stringset.go \
	src/onos/util.go

bp2/hooks/onos-hook: \
	src/hook/gather.go \
	src/hook/onos-hook.go \
	src/onos/onos.go \
	src/onos/stringset.go \
	src/onos/util.go
