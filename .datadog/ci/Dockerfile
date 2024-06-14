FROM registry.ddbuild.io/images/mirror/golang:1.22.4

ARG TARGETARCH

RUN apt-get update && \
  apt-get install -y --no-install-recommends \
  ca-certificates \
  curl \
  gnupg2 \
  unzip \
  jq \
  && rm -rf /var/lib/apt/lists/*

# Install Datadog default internal certificates
COPY --from=registry.ddbuild.io/images/datadog-ca-certs:standard /certs/ /usr/local/share/ca-certificates/
RUN set -x \
    && chmod -R o-w /usr/local/share/ca-certificates \
    && update-ca-certificates

# install docker cli
RUN set -x \
  && curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc \
  && chmod a+r /etc/apt/keyrings/docker.asc \
  && echo "deb [arch=$TARGETARCH signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu focal stable" > /etc/apt/sources.list.d/docker.list \
  && apt-get update \
  && apt-get install -y --no-install-recommends docker-ce-cli=5:26.1.4-1~ubuntu.20.04~focal docker-buildx-plugin=0.14.1-1~ubuntu.20.04~focal \
  && rm -rf /var/lib/apt/lists/*

# Install EEE registry auth helper
RUN set -x \
  && DIGEST=$(curl -H "Accept: application/vnd.oci.image.manifest.v1+json" https://registry.ddbuild.io/v2/public/setup-docker-credentials/manifests/$TARGETARCH | jq -r '.layers[0].digest') \
  && curl -L https://registry.ddbuild.io/v2/public/setup-docker-credentials/blobs/$DIGEST > /usr/bin/setup-docker-credentials \
  && bash /usr/bin/setup-docker-credentials

RUN rm -rf docker-helpers

# Install ddsign for image signing
# https://datadoghq.atlassian.net/wiki/spaces/SECENG/pages/2744681107/Image+Integrity+User+Guide
COPY --from=registry.ddbuild.io/ddsign:v1.8.1@sha256:e33a5c3c1036111d98e2dd0bcbdb3e29dee7290b89c6384d90737e8700f53247 /usr/local/bin/ddsign /usr/local/bin/ddsign
