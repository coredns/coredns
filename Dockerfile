FROM --platform=linux/$BUILDARCH k8s.gcr.io/build-image/setcap:buster-v1.4.0 AS setcap
COPY coredns /coredns
# We apply cap_net_bind_service so that coredns can be run as
# non-root and still listen on port less than 1024
RUN setcap cap_net_bind_service=+ep /coredns

FROM gcr.io/distroless/static:nonroot
COPY --from=setcap /coredns /coredns
USER nonroot:nonroot
EXPOSE 53 53/udp
ENTRYPOINT ["/coredns"]
