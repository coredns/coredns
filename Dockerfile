ARG DEBIAN_IMAGE=debian:stable-slim
ARG BASE=scratch
FROM --platform=$BUILDPLATFORM ${DEBIAN_IMAGE}
SHELL [ "/bin/sh", "-ec" ]

RUN export DEBCONF_NONINTERACTIVE_SEEN=true \
           DEBIAN_FRONTEND=noninteractive \
           DEBIAN_PRIORITY=critical \
           TERM=linux ; \
    apt-get -qq update ; \
    apt-get -yyqq upgrade ; \
    apt-get -yyqq install ca-certificates ; \
    apt-get clean

FROM --platform=$TARGETPLATFORM ${BASE}

COPY --from=0 /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ADD coredns /coredns

EXPOSE 53 53/udp
ENTRYPOINT ["/coredns"]
