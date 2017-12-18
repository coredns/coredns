FROM alpine:latest

ADD coredns.sh /coredns.sh
ADD coredns /coredns

EXPOSE 53 53/udp
ENTRYPOINT ["/coredns.sh"]
