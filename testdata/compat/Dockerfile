FROM docker.io/library/golang:1.25.5-bookworm

# hadolint ignore=DL3008
RUN set -xe;\
    apt-get update; \
    apt-get install -y --no-install-recommends liblzo2-dev; \
    apt-get clean; \
    rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY . .

USER nobody:nogroup
ENV GOCACHE=/tmp/go-build \
    GOMODCACHE=/tmp/go-mod

ENTRYPOINT ["make"]
CMD ["test-compat"]
