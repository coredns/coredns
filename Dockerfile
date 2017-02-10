FROM alpine:latest
MAINTAINER Miek Gieben <miek@miek.nl> @miekg

RUN apk --update add bind-tools && rm -rf /var/cache/apk/*

COPY coredns /coredns
COPY docker-entrypoint.sh /docker-entrypoint.sh

EXPOSE 53 53/udp
ENTRYPOINT ["/docker-entrypoint.sh"]
