#BUILD_VERBOSE :=
BUILD_VERBOSE := -v

TEST_VERBOSE :=
TEST_VERBOSE := -v

DOCKER_IMAGE_NAME := $$USER/coredns


all:
	GOOS=linux go build -a -tags netgo -installsuffix netgo
	# Build static binary below. This might not be needed?
	#CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo

.PHONY: docker
docker: coredns
	docker build -t $(DOCKER_IMAGE_NAME) .

.PHONY: deps
deps:
	go get ${BUILD_VERBOSE}

.PHONY: test
test:
	go test $(TEST_VERBOSE) ./...

.PHONY: testk8s
testk8s:
	go test $(TEST_VERBOSE) -tags=k8s -run 'TestK8sIntegration' ./test

.PHONY: clean
clean:
	go clean
