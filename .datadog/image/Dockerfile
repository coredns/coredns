FROM registry.ddbuild.io/images/base/gbi-ubuntu_2204:release
ARG TARGETARCH
ARG TARGETOS

USER root

RUN apt-get update && apt-get install -y libcap2-bin

COPY coredns-${TARGETOS}-${TARGETARCH} /usr/local/bin/coredns
RUN setcap 'cap_net_bind_service=+ep' /usr/local/bin/coredns

RUN rm -rf /var/lib/apt/lists/* \
  && apt-get remove -y libcap2-bin

USER nobody

ENTRYPOINT ["/usr/local/bin/coredns"]
