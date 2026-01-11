# Makefile for building CoreDNS
GITCOMMIT?=$(shell git describe --dirty --always)
BINARY:=coredns
SYSTEM:=
CHECKS:=check
BUILDOPTS?=-v
GOPATH?=$(HOME)/go
MAKEPWD:=$(dir $(realpath $(firstword $(MAKEFILE_LIST))))
CGO_ENABLED?=0
GOLANG_VERSION ?= $(shell cat .go-version)
STRIP_FLAGS?=-s -w
LDFLAGS?=-ldflags="$(STRIP_FLAGS) -X github.com/coredns/coredns/coremain.GitCommit=$(GITCOMMIT)"

export GOSUMDB = sum.golang.org
export GOTOOLCHAIN = go$(GOLANG_VERSION)

.PHONY: all
all: coredns

.PHONY: coredns
coredns: $(CHECKS)
	CGO_ENABLED=$(CGO_ENABLED) $(SYSTEM) go build $(BUILDOPTS) $(LDFLAGS) -o $(BINARY)

.PHONY: check
check: core/plugin/zplugin.go core/dnsserver/zdirectives.go

core/plugin/zplugin.go core/dnsserver/zdirectives.go: plugin.cfg
	go generate coredns.go
	go get

.PHONY: gen
gen:
	go generate coredns.go
	go get

.PHONY: pb
pb:
	$(MAKE) -C pb

.PHONY: clean
clean:
	go clean
	rm -f coredns
