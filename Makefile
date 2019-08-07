# Makefile for building CoreDNS
GITCOMMIT:=$(shell git describe --dirty --always)
BINARY:=coredns
SYSTEM:=
CHECKS:=check
BUILDOPTS:=-v
GOPATH?=$(HOME)/go
PRESUBMIT:=core coremain plugin test request
MAKEPWD:=$(dir $(realpath $(firstword $(MAKEFILE_LIST))))
CGO_ENABLED:=0

.PHONY: all
all: coredns

.PHONY: coredns
coredns: $(CHECKS)
	GO111MODULE=on CGO_ENABLED=$(CGO_ENABLED) $(SYSTEM) go build $(BUILDOPTS) -ldflags="-s -w -X github.com/coredns/coredns/coremain.GitCommit=$(GITCOMMIT)" -o $(BINARY)

.PHONY: check
check: presubmit core/plugin/zplugin.go core/dnsserver/zdirectives.go

.PHONY: travis
travis:
ifeq ($(TEST_TYPE),core)
	( cd request ; GO111MODULE=on go test -v -race ./... )
	( cd core ; GO111MODULE=on go test -v -race  ./... )
	( cd coremain ; GO111MODULE=on go test -v -race ./... )
endif
ifeq ($(TEST_TYPE),integration)
	( cd test ; GO111MODULE=on go test -v -race ./... )
endif
ifeq ($(TEST_TYPE),plugin)
	( cd plugin ; GO111MODULE=on go test -v -race ./... )
endif
ifeq ($(TEST_TYPE),coverage)
	for d in `go list ./... | grep -v vendor`; do \
		t=$$(date +%s); \
		GO111MODULE=on go test -i -coverprofile=cover.out -covermode=atomic $$d || exit 1; \
		GO111MODULE=on go test -v -coverprofile=cover.out -covermode=atomic $$d || exit 1; \
		echo "Coverage test $$d took $$(($$(date +%s)-t)) seconds"; \
		if [ -f cover.out ]; then \
			cat cover.out >> coverage.txt; \
			rm cover.out; \
		fi; \
	done
endif
ifeq ($(TEST_TYPE),fuzzit)
	export GO111MODULE=off
	go get -u github.com/dvyukov/go-fuzz/go-fuzz-build
	go get -u ./...
	LIBFUZZER=YES make -f Makefile.fuzz cache chaos file rewrite whoami
	wget -O fuzzit https://github.com/fuzzitdev/fuzzit/releases/download/v2.4.8/fuzzit_Linux_x86_64
	chmod a+x fuzzit
	./fuzzit auth $(FUZZIT_API_KEY)
	./fuzzit create job --branch $(TRAVIS_BRANCH) --revision $(TRAVIS_COMMIT) cache ./cache
	./fuzzit create job --branch $(TRAVIS_BRANCH) --revision $(TRAVIS_COMMIT) chaos ./chaos
	./fuzzit create job --branch $(TRAVIS_BRANCH) --revision $(TRAVIS_COMMIT) file ./file
	./fuzzit create job --branch $(TRAVIS_BRANCH) --revision $(TRAVIS_COMMIT) rewrite ./rewrite
	./fuzzit create job --branch $(TRAVIS_BRANCH) --revision $(TRAVIS_COMMIT) whoami ./whoami
endif
ifeq ($(TEST_TYPE),fuzzit-regression)
	export GO111MODULE=off
	go get -u github.com/dvyukov/go-fuzz/go-fuzz-build
	LIBFUZZER=YES make -f Makefile.fuzz cache chaos file rewrite whoami
	wget -O fuzzit https://github.com/fuzzitdev/fuzzit/releases/download/v2.4.8/fuzzit_Linux_x86_64
	chmod a+x fuzzit
	./fuzzit create job --local coredns/cache ./cache
	./fuzzit create job --local coredns/chaos ./chaos
	./fuzzit create job --local coredns/file ./file
	./fuzzit create job --local coredns/rewrite ./rewrite
	./fuzzit create job --local coredns/whoami ./whoami
endif

core/plugin/zplugin.go core/dnsserver/zdirectives.go: plugin.cfg
	GO111MODULE=on go generate coredns.go

.PHONY: gen
gen:
	GO111MODULE=on go generate coredns.go

.PHONY: pb
pb:
	$(MAKE) -C pb

# Presubmit runs all scripts in .presubmit; any non 0 exit code will fail the build.
.PHONY: presubmit
presubmit:
	@for pre in $(MAKEPWD)/.presubmit/* ; do "$$pre" $(PRESUBMIT) || exit 1 ; done

.PHONY: clean
clean:
	GO111MODULE=on go clean
	rm -f coredns
