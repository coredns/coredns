FROM golang:1.23 AS builder

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dns-operator ./cmd/operator/

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /workspace/dns-operator /dns-operator
USER 65532:65532
ENTRYPOINT ["/dns-operator"]
