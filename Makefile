mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
top := $(dir $(mkfile_path))

all: image

image: docker.img

docker.img: bp2/hooks/onos-hook bp2/hooks/onos-wrapper bp2/hooks/onos-service Dockerfile
	docker build --tag=ciena/onos:1.3  .
	touch docker.img

bp2/hooks/onos-wrapper: src/github.com/ciena/wrapper/wrapper.go
	GOPATH=$(top)/vendor:$(top) CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO15VENDOREXPERIMENT=1 \
		go build -o bp2/hooks/onos-wrapper github.com/ciena/wrapper

src/github.com/ciena/wrapper/wrapper.go: src/github.com/ciena/wrapper

src/github.com/ciena/wrapper: #pkg/linux_amd64/github.com/davidkbainbridge/jsonq.a
	mkdir -p $(top)/src/github.com/ciena
	ln -s $(top)wrapper $(top)src/github.com/ciena/wrapper

bp2/hooks/onos-hook: src/github.com/ciena/hook/gather.go src/github.com/ciena/hook/onos-hook.go
	GOPATH=$(top)/vendor:$(top) CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO15VENDOREXPERIMENT=1 \
		go build -o bp2/hooks/onos-hook github.com/ciena/hook

src/github.com/ciena/hook/gather.go: src/github.com/ciena/hook

src/github.com/ciena/hook/onos-hook.go: src/github.com/ciena/hook

src/github.com/ciena/onos:
	mkdir -p $(top)/src/github.com/ciena
	ln -s $(top)onos $(top)src/github.com/ciena/onos

src/github.com/ciena/hook:
	mkdir -p $(top)/src/github.com/ciena/
	ln -s $(top)hook $(top)src/github.com/ciena/hook

clean:
	rm -rf docker.img *~ bp2/hooks/onos-hook bp2/hooks/onos-wrapper bin pkg src
