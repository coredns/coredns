FROM gcr.io/distroless/static:nonroot

ADD coredns /coredns

USER nonroot:nonroot
EXPOSE 53 53/udp
ENTRYPOINT ["/coredns"]
