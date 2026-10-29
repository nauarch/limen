# limen: a static binary on an empty image, with the CA bundle it needs to reach registries over
# TLS and the limenctl binaries it hands out at /setup built in. It runs as an unprivileged user
# and keeps its store in /var/lib/limen. The build cross-compiles, so a multi-platform build
# (docker buildx build --platform linux/amd64,linux/arm64) needs no emulation; VERSION stamps
# limen and the limenctl it carries.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN LIMEN_VERSION=$VERSION go generate ./internal/frontdoor/setup/dist \
 && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o /out/limen ./cmd/limen \
 && mkdir -p /out/store

FROM scratch
LABEL org.opencontainers.image.title="limen" \
      org.opencontainers.image.description="A pull-through cache for container registries" \
      org.opencontainers.image.source="https://github.com/nauarch/limen" \
      org.opencontainers.image.authors="darki73" \
      org.opencontainers.image.licenses="Apache-2.0"
COPY LICENSE NOTICE /
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/limen /limen
COPY --from=build --chown=65532:65532 /out/store /var/lib/limen
USER 65532:65532
EXPOSE 5000
VOLUME /var/lib/limen
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 CMD ["/limen", "--healthcheck"]
ENTRYPOINT ["/limen", "--dir=/var/lib/limen"]
