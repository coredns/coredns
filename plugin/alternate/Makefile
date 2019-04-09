VERSION:=0.1
TAG:=v$(VERSION)

COVEROUT = cover.out
GOFMTCHECK = test -z `gofmt -l -s -w *.go | tee /dev/stderr`
GOTEST = go test -v
COVER = $(GOTEST) -coverprofile=$(COVEROUT) -covermode=atomic -race
GOPATH?=$(HOME)/go

all: get fmt test

.PHONY: fmt
fmt:
	@echo "Checking format..."
	@$(GOFMTCHECK)

.PHONY: get
get:
	@ # Not vendoring these, so external plugins compile, avoiding:
	@ # cannot use c (type *"github.com/mholt/caddy".Controller) as type
	@ # *"github.com/coredns/coredns/vendor/github.com/mholt/caddy".Controller like errors.
	(cd $(GOPATH)/src/github.com/mholt/caddy 2>/dev/null              && git checkout -q master 2>/dev/null || true)
	(cd $(GOPATH)/src/github.com/miekg/dns 2>/dev/null                && git checkout -q master 2>/dev/null || true)
	(cd $(GOPATH)/src/github.com/prometheus/client_golang 2>/dev/null && git checkout -q master 2>/dev/null || true)
	go get -u github.com/mholt/caddy
	go get -u github.com/miekg/dns
	go get -u github.com/prometheus/client_golang/prometheus/promhttp
	go get -u github.com/prometheus/client_golang/prometheus
	(cd $(GOPATH)/src/github.com/mholt/caddy              && git checkout -q v0.11.1)
	(cd $(GOPATH)/src/github.com/miekg/dns                && git checkout -q v1.1.4)
	(cd $(GOPATH)/src/github.com/prometheus/client_golang && git checkout -q v0.9.1)
	go get -v

.PHONY: test
test:
	@echo "Running tests..."
	@$(COVER)

# Use the 'release' target to start a release
.PHONY: release
release: commit push
	@echo Released $(VERSION)

.PHONY: commit
commit:
	@echo Committing release $(VERSION)
	git commit -am"Release $(VERSION)"
	git tag $(TAG)

.PHONY: push
push:
	@echo Pushing release $(VERSION) to master
	git push --tags
	git push
