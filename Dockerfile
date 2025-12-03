# syntax=docker/dockerfile:1

###############################################
# Frontend build stage (mirrors ui-build job) #
###############################################
FROM --platform=$BUILDPLATFORM node:22-alpine AS ui-builder

WORKDIR /src/ui

# 启用 corepack（默认可能 disable）
RUN corepack enable

# 指定 yarn 版本（1.22.22 为例）
RUN corepack prepare yarn@1.22.22 --activate

COPY ui/package.json ui/yarn.lock ./
RUN yarn install

COPY ui ./
RUN yarn run build-only

################################################
# Backend build stage (mirrors backend-build)  #
################################################
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS go-builder
WORKDIR /src
ARG TARGETOS
ARG TARGETARCH
ENV GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    CGO_ENABLED=0

COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui-builder /src/ui/dist ./ui/dist
RUN go build -trimpath -ldflags "-s -w" -o /out/sealchat-server .

#############################
# Minimal Alpine runtime    #
#############################
FROM alpine:3.20
WORKDIR /app
RUN addgroup -S sealchat && adduser -S -G sealchat sealchat \
 && apk add --no-cache ca-certificates tzdata ffmpeg \
 && mkdir -p /app/data /app/static /app/temp \
 && chown -R sealchat:sealchat /app
COPY --from=go-builder /out/sealchat-server /usr/local/bin/sealchat-server
COPY config.yaml.example /app/config.yaml.example
EXPOSE 3212
VOLUME ["/app/data", "/app/static", "/app/temp"]
USER sealchat
ENTRYPOINT ["/usr/local/bin/sealchat-server"]
