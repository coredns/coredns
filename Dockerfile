ARG DEBIAN_IMAGE=debian:stable-slim
ARG BASE=gcr.io/distroless/static-debian12:nonroot

FROM --platform=$BUILDPLATFORM ${DEBIAN_IMAGE} AS build
ARG DEBIAN_FRONTEND=noninteractive
RUN apt-get -qq update \
    && apt-get -qq --no-install-recommends install libcap2-bin
COPY coredns /coredns
RUN setcap cap_net_bind_service=+ep /coredns

FROM ${BASE}
COPY --from=build /coredns /coredns
# Use the numeric uid/gid (distroless "nonroot" is 65532) rather than the named
# user so Kubernetes can verify the image runs as non-root when runAsNonRoot is
# set. A named user is rejected at admission ("container has runAsNonRoot and
# image has non-numeric user, cannot verify user is non-root"). The binary keeps
# cap_net_bind_service (set in the build stage), so it can still bind :53.
# Refs #7542.
USER 65532:65532
# Reset the working directory inherited from the base image back to the expected default:
# https://github.com/coredns/coredns/issues/7009#issuecomment-3124851608
WORKDIR /
EXPOSE 53 53/udp
ENTRYPOINT ["/coredns"]
