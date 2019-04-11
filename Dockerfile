FROM gcr.io/distroless/static:latest
ADD coredns /coredns

EXPOSE 53 53/udp
ENTRYPOINT ["/coredns"]
