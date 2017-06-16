FROM alpine:latest
MAINTAINER Miek Gieben <miek@miek.nl> @miekg

RUN apk --update add bind-tools && rm -rf /var/cache/apk/*

# need to uncomment if want to use https_google
#RUN apk update && apk add ca-certificates && update-ca-certificates && apk add openssl

ADD coredns /coredns

EXPOSE 53 53/udp
ENTRYPOINT ["/coredns"]
