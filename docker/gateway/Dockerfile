ARG GOLANG_IMAGE=golang:1.25-alpine
ARG ALPINE_IMAGE=alpine:3.21

# Build natively on the builder platform and cross-compile for the target.
FROM --platform=$BUILDPLATFORM ${GOLANG_IMAGE} AS build
ARG GOPROXY=https://proxy.golang.org,direct
ARG GOSUMDB=sum.golang.org
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ENV GOPROXY=${GOPROXY} GOSUMDB=${GOSUMDB}
WORKDIR /src
COPY packages/gateway/go.mod packages/gateway/go.sum ./
RUN go mod download
COPY packages/gateway/ ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /gateway ./cmd/gateway

FROM ${ALPINE_IMAGE}
RUN apk add --no-cache ca-certificates \
    && adduser -D -H -u 10001 gateway
COPY --from=build /gateway /usr/local/bin/gateway
USER 10001
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/health >/dev/null || exit 1
ENTRYPOINT ["gateway"]
