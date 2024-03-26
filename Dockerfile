ARG BASE=gcr.io/distroless/static-debian11:nonroot

FROM --platform=$TARGETPLATFORM ${BASE}
COPY coredns /coredns
USER nonroot:nonroot
EXPOSE 53 53/udp
ENTRYPOINT ["/coredns"]
