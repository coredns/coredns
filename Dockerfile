ARG BASE=gcr.io/distroless/static-debian12:nonroot
FROM ${BASE}
COPY coredns /coredns

USER nonroot:nonroot
# Reset the working directory inherited from the base image back to the expected default:
# https://github.com/coredns/coredns/issues/7009#issuecomment-3124851608
WORKDIR /
EXPOSE 53 53/udp
ENTRYPOINT ["/coredns"]
