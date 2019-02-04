FROM debian:stable-slim

RUN apt-get update && apt-get -uy upgrade
RUN apt-get -y install ca-certificates && update-ca-certificates

FROM scratch
# Allow Glog to log to disk
COPY --from=base /tmp /tmp
COPY --from=0 /etc/ssl/certs /etc/ssl/certs

ADD coredns /coredns

EXPOSE 53 53/udp
ENTRYPOINT ["/coredns"]
