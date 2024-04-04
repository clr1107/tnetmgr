# Used this as a base: https://github.com/azer/go-makefile-example

BINARY=tnetmgr
VERSION=0.0.1
BUILD=`git rev-parse --short HEAD`
PLATFORMS=linux
ARCHITECTURES=386 amd64

LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.Build=${BUILD}"

default: build

all: clean build_all install

build:
	go build ${LDFLAGS} -o bin/${BINARY} ./cmd/main.go

build_all:
	$(foreach GOOS, $(PLATFORMS),\
	$(foreach GOARCH, $(ARCHITECTURES), $(shell export GOOS=$(GOOS); export GOARCH=$(GOARCH); go build -o bin/$(BINARY)-$(GOARCH)-$(BUILD) ./cmd/main.go)))

install:
	go install ${LDFLAGS}

clean:
	rm bin/*

.PHONY: check clean install build_all all