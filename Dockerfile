#build stage
FROM golang:1.16 AS builder
RUN mkdir -p /go/src/app
COPY go.sum go.mod /go/src/app/
WORKDIR /go/src/app
RUN go mod download

COPY . /go/src/app
RUN make

FROM debian:stable-slim AS slim

RUN apt-get update && apt-get -uy upgrade
RUN apt-get -y install ca-certificates && update-ca-certificates

FROM scratch
COPY --from=slim /etc/ssl/certs /etc/ssl/certs
WORKDIR /
COPY --from=builder /go/src/app/coredns /coredns

EXPOSE 53 53/udp
ENTRYPOINT ["/coredns"]
