# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM node:20-bookworm-slim AS ui-builder
WORKDIR /src/ui
COPY ui/ ./
COPY doc/template/ /src/doc/template/
RUN --mount=type=cache,target=/root/.cache/yarn \
  if [ -f dist/index.html ]; then echo "Using prebuilt ui/dist from build context"; else corepack enable && yarn install --network-timeout 600000 && yarn build-only; fi

FROM --platform=$BUILDPLATFORM golang:1.24-bookworm AS go-builder
WORKDIR /src

ENV GOMODCACHE=/go/pkg/mod \
  GOCACHE=/root/.cache/go-build

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  go mod download

COPY . ./
COPY --from=ui-builder /src/ui/dist ./ui/dist

ARG TARGETOS
ARG TARGETARCH
ARG BUILD_VERSION
RUN --mount=type=cache,target=/root/.cache/go-build \
  test -n "${TARGETOS}" && test -n "${TARGETARCH}" && \
  CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -o /out/sealchat-server -trimpath -buildvcs=false -ldflags "-s -w -X sealchat/utils.BuildVersion=${BUILD_VERSION}" .

FROM --platform=$BUILDPLATFORM alpine:3.20 AS webp-assets
ARG TARGETARCH
WORKDIR /src
COPY bin/ ./bin/
RUN set -eux; \
  test -n "${TARGETARCH}"; \
  case "${TARGETARCH}" in \
    amd64) WEBP_DIR="linux-x64" ;; \
    arm64) WEBP_DIR="linux-arm64" ;; \
    *) echo "unsupported TARGETARCH=${TARGETARCH}"; exit 1 ;; \
  esac; \
  mkdir -p /out/bin/"${WEBP_DIR}"; \
  install -m 0755 ./bin/"${WEBP_DIR}"/cwebp /out/bin/"${WEBP_DIR}"/cwebp; \
  install -m 0755 ./bin/"${WEBP_DIR}"/gif2webp /out/bin/"${WEBP_DIR}"/gif2webp; \
  cp ./bin/LICENSE /out/LICENSE; \
  test -x /out/bin/"${WEBP_DIR}"/cwebp; \
  test -x /out/bin/"${WEBP_DIR}"/gif2webp

FROM alpine:3.20
ARG TARGETARCH
RUN apk add --no-cache ca-certificates tzdata wget ffmpeg && update-ca-certificates
WORKDIR /app
ARG TARGETARCH

COPY --from=go-builder /out/sealchat-server /app/sealchat-server
COPY --from=webp-assets /out/bin /app/bin
COPY --from=webp-assets /out/LICENSE /app/LICENSE
RUN set -eux; \
  test -n "${TARGETARCH}"; \
  case "${TARGETARCH}" in \
    amd64) WEBP_DIR="linux-x64" ;; \
    arm64) WEBP_DIR="linux-arm64" ;; \
    *) echo "unsupported TARGETARCH=${TARGETARCH}"; exit 1 ;; \
  esac; \
  chmod +x /app/bin/"${WEBP_DIR}"/cwebp /app/bin/"${WEBP_DIR}"/gif2webp

EXPOSE 3212
ENTRYPOINT ["/app/sealchat-server"]
