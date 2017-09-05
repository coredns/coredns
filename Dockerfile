FROM alpine:latest
MAINTAINER Miek Gieben <miek@miek.nl> @miekg

ARG VERSION=010

RUN set -ex &&\
    apk add --no-cache --virtual .build curl gzip libcap &&\

    # only need ca-certificates & openssl if want to use https_google
    apk add --no-cache bind-tools ca-certificates openssl &&\
    update-ca-certificates &&\
    curl -L https://github.com/coredns/coredns/releases/download/v${VERSION}/coredns_${VERSION}_linux_x86_64.tgz | tar xzvf - &&\
    adduser -S coredns &&\
    chown coredns /coredns &&\
    setcap cap_net_bind_service=+ep /coredns &&\

    apk del --no-cache --purge .build

USER coredns

EXPOSE 53 53/udp

ENTRYPOINT ["/coredns"]
