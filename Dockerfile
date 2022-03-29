FROM debian:stable-slim@sha256:a04e8ec615db03154299695a65f7a85b8800a4e7878d241871419d3878e3662d

RUN apt-get update && apt-get -uy upgrade
RUN apt-get -y install ca-certificates && update-ca-certificates

FROM scratch

COPY --from=0 /etc/ssl/certs /etc/ssl/certs
ADD coredns /coredns

EXPOSE 53 53/udp
ENTRYPOINT ["/coredns"]
