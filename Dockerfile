FROM alpine:latest

# first add the file because we have to run setcap on the binary and we would not like to create a new image layer so the binary must exist on RUN
ADD coredns /coredns

# Only need ca-certificates & openssl if want to use DNS over TLS (RFC 7858).
RUN apk --no-cache add bind-tools ca-certificates openssl libcap && update-ca-certificates && chown nobody:nobody /coredns && setcap CAP_NET_BIND_SERVICE=+eip /coredns

USER nobody

EXPOSE 53 53/udp

ENTRYPOINT ["/coredns"]
