# Makefile for building CoreDNS
GITCOMMIT:=$(shell git describe --dirty --always)
BINARY:=coredns
SYSTEM:=
CHECKS:=check
BUILDOPTS:=-v
GOPATH?=$(HOME)/go
MAKEPWD:=$(dir $(realpath $(firstword $(MAKEFILE_LIST))))
CGO_ENABLED:=0

.PHONY: all
all: coredns

.PHONY: coredns
coredns: $(CHECKS)
	CGO_ENABLED=$(CGO_ENABLED) $(SYSTEM) go build $(BUILDOPTS) -ldflags="-s -w -X github.com/coredns/coredns/coremain.GitCommit=$(GITCOMMIT)" -o $(BINARY)

.PHONY: check
check: core/plugin/zplugin.go core/dnsserver/zdirectives.go

.PHONY: travis
travis:
ifeq ($(TEST_TYPE),core)
	( cd request; go test -race ./... )
	( cd core; go test -race  ./... )
	( cd coremain; go test -race ./... )
endif
ifeq ($(TEST_TYPE),integration)
	( cd test; go test -race ./... )
endif
ifeq ($(TEST_TYPE),fmt)
	( echo "fmt"; gofmt -w -s . | grep ".*\.go"; if [ "$$?" = "0" ]; then exit 1; fi )
endif
ifeq ($(TEST_TYPE),metrics)
	( echo "metrics"; go get github.com/fatih/faillint)
	( faillint -paths "github.com/prometheus/client_golang/prometheus.{NewCounter,NewCounterVec,NewCounterVec,\
	NewGauge,NewGaugeVec,NewGaugeFunc,NewHistorgram,NewHistogramVec,NewSummary,NewSummaryVec}=github.com/prometheus/client_golang/prometheus/promauto.{NewCounter,\
	NewCounterVec,NewCounterVec,NewGauge,NewGaugeVec,NewGaugeFunc,NewHistorgram,NewHistogramVec,NewSummary,NewSummaryVec}" ./...)
endif
ifeq ($(TEST_TYPE),plugin)
	( cd plugin; go test -race ./... )
endif
ifeq ($(TEST_TYPE),coverage)
	for d in `go list ./... | grep -v vendor`; do \
		t=$$(date +%s); \
		go test -i -coverprofile=cover.out -covermode=atomic $$d || exit 1; \
		go test -coverprofile=cover.out -covermode=atomic $$d || exit 1; \
		if [ -f cover.out ]; then \
			cat cover.out >> coverage.txt && rm cover.out; \
		fi; \
	done
endif
ifeq ($(TEST_TYPE),fuzzit)
	# skip fuzzing for PR
	if [ "$(TRAVIS_PULL_REQUEST)" = "false" ] || [ "$(FUZZIT_TYPE)" = "local-regression" ] ; then \
		export GO111MODULE=off; \
		go get -u github.com/dvyukov/go-fuzz/go-fuzz-build; \
		go get -u -v .; \
		cd ../../go-acme/lego && git checkout v2.5.0; \
		cd ../../coredns/coredns; \
		LIBFUZZER=YES $(MAKE) -f Makefile.fuzz all; \
		$(MAKE) -sf Makefile.fuzz fuzzit; \
		for i in `$(MAKE) -sf Makefile.fuzz echo`; do echo $$i; \
			./fuzzit create job --type $(FUZZIT_TYPE) coredns/$$i ./$$i; \
		done; \
	fi;
endif

core/plugin/zplugin.go core/dnsserver/zdirectives.go: plugin.cfg
	go generate coredns.go

.PHONY: gen
gen:
	go generate coredns.go

.PHONY: pb
pb:
	$(MAKE) -C pb

.PHONY: clean
clean:
	go clean
	rm -f coredns
