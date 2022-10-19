BINARY_NAME=planner
SCALEOUT_PLUGIN=scale_out
RMPOD_PLUGIN=rm_pod
RDT_PLUGIN=rdt
GO_CILINT_CHECKERS=errcheck,goimports,gosec,gosimple,govet,ineffassign,nilerr,revive,staticcheck,unused
DOCKER_IMAGE_VERSION=0.1.0

api:
	hack/generate_code.sh

golangci-lint:
	golangci-lint run --color always -E ${GO_CILINT_CHECKERS} --timeout=5m -v ./...

proto:
	hack/generate_protobuf.sh

gen_code: api proto

build:
	CGO_ENABLED=0 go build -o bin/${BINARY_NAME} cmd/main.go

build-plugins:
	CGO_ENABLED=0 go build -o bin/plugins/${SCALEOUT_PLUGIN} plugins/${SCALEOUT_PLUGIN}/cmd/${SCALEOUT_PLUGIN}.go
	CGO_ENABLED=0 go build -o bin/plugins/${RMPOD_PLUGIN} plugins/${RMPOD_PLUGIN}/cmd/${RMPOD_PLUGIN}.go
	CGO_ENABLED=0 go build -o bin/plugins/${RDT_PLUGIN} plugins/${RDT_PLUGIN}/cmd/${RDT_PLUGIN}.go

controller-images:
	docker build -t planner:${DOCKER_IMAGE_VERSION} .

plugin-images:
	docker build -t scaleout:${DOCKER_IMAGE_VERSION} -f plugins/scale_out/Dockerfile .
	docker build -t rmpod:${DOCKER_IMAGE_VERSION} -f plugins/rm_pod/Dockerfile .
	docker build -t rdt:${DOCKER_IMAGE_VERSION} -f plugins/rdt/Dockerfile .

all-images: controller-images plugin-images

all: gen_code build build-plugins

run:
	./bin/${BINARY_NAME}

build_and_run: build run

prepare-build:
	go mod download
	go mod tidy

utest:
	go test -count=1 -v ./...

test:
	hack/run_test.sh

benchmark:
	go test -bench=. ./... -run=^$

clean:
	go clean --cache
	rm -rf bin/*
