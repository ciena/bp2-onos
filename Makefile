mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
top := $(dir $(mkfile_path))

all: image

image: docker.img

docker.img: bp2/hooks/onos-hook bp2/hooks/onos-wrapper bp2/hooks/onos-service Dockerfile
	docker build --tag=cyan/onos:1.3  .
	touch docker.img

bp2/hooks/onos-wrapper: src/github.com/ciena/wrapper/wrapper.go
	GOPATH=$(top) CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -o bp2/hooks/onos-wrapper github.com/ciena/wrapper

src/github.com/ciena/wrapper/wrapper.go: src/github.com/ciena/wrapper

src/github.com/ciena/wrapper:
	mkdir -p $(top)/src/github.com/ciena
	ln -s $(top)wrapper $(top)src/github.com/ciena/wrapper

bp2/hooks/onos-hook: src/github.com/ciena/onos/gather.go src/github.com/ciena/onos/onos-hook.go
	GOPATH=$(top) CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -o bp2/hooks/onos-hook github.com/ciena/onos

src/github.com/ciena/onos/gather.go: src/github.com/ciena/onos

src/github.com/ciena/onos/onos-hook.go: src/github.com/ciena/onos

src/github.com/ciena/onos:
	mkdir -p $(top)/src/github.com/ciena
	ln -s $(top)onos $(top)src/github.com/ciena/onos

foo:
	mkdir -p $(top)/src/github.com/ciena
	ln -s $(top)wrapper $(top)src/github.com/ciena/wrapper
	ln -s $(top)onos $(top)src/github.com/ciena/onos

clean:
	rm -rf docker.img *~ bp2/hooks/onos-hook bp2/hooks/onos-wrapper src
