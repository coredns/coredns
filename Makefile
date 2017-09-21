GITCOMMIT:=$(shell git describe --dirty --always)
BINARY:=coredns
SYSTEM:=

all: coredns

# Phony this to ensure we always build the binary.
# TODO: Add .go file dependencies.
.PHONY: coredns
coredns: check godeps
	CGO_ENABLED=0 $(SYSTEM) go build -v -ldflags="-s -w -X github.com/coredns/coredns/coremain.gitCommit=$(GITCOMMIT)" -o $(BINARY)

.PHONY: check
check: linter core/zplugin.go core/dnsserver/zdirectives.go godeps

.PHONY: test
test: check
	go test -race -v ./test ./plugin/...

.PHONY: testk8s
testk8s: check
	go test -race -v -tags=k8s ./test/kubernetes ./plugin/kubernetes/...

.PHONY: godeps
godeps:
	go get github.com/mholt/caddy
	go get github.com/miekg/dns
	go get golang.org/x/net/context
	go get golang.org/x/text

.PHONY: travis
travis: check
ifeq ($(TEST_TYPE),core)
	( cd request ; go test -v -race ./... )
	( cd core ; go test -v -race  ./... )
	( cd coremain go test -v -race ./... )
endif
ifeq ($(TEST_TYPE),integration)
	( go test -v -tags 'etcd' -race ./test )
endif
ifeq ($(TEST_TYPE),integration-k8s1)
	( go test -v -tags 'k8s1' -race ./test/kubernetes )
endif
ifeq ($(TEST_TYPE),integration-k8s2)
	( go test -v -tags 'k8s2' -race ./test/kubernetes )
endif
ifeq ($(TEST_TYPE),integration-k8sexclust)
	( go test -v -tags 'k8sexclust' -race ./test/kubernetes )
endif
ifeq ($(TEST_TYPE),plugin)
	( cd plugin ; go test -v -race ./... )
endif
ifeq ($(TEST_TYPE),coverage)
	for d in `go list ./... | grep -v vendor | grep -v coredns\/test`; do \
		t=$$(date +%s); \
		go test -i -coverprofile=cover.out -covermode=atomic $$d || exit 1; \
		go test -v -coverprofile=cover.out -covermode=atomic $$d || exit 1; \
		echo "Coverage test $$d took $$(($$(date +%s)-t)) seconds"; \
		if [ -f cover.out ]; then \
			cat cover.out >> coverage.txt; \
			rm cover.out; \
		fi; \
	done
endif


core/zplugin.go core/dnsserver/zdirectives.go: plugin.cfg
	go generate coredns.go

.PHONY: gen
gen:
	go generate coredns.go

.PHONY: linter
linter:
	go get -u github.com/alecthomas/gometalinter
	gometalinter --install golint
	gometalinter --deadline=1m --disable-all --enable=gofmt --enable=golint --enable=vet --exclude=^vendor/ --exclude=^pb/ ./...

.PHONY: clean
clean:
	go clean
	rm -f coredns
