ARG TARGETPLATFORM=linux/amd64
ARG GOLANG_IMAGE=golang:1.25-alpine
ARG ALPINE_IMAGE=alpine:3.21

FROM --platform=${TARGETPLATFORM} ${GOLANG_IMAGE} AS build
ARG GOPROXY=https://proxy.golang.org,direct
ARG GOSUMDB=sum.golang.org
ENV GOPROXY=${GOPROXY} GOSUMDB=${GOSUMDB}
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY packages/gateway/go.mod packages/gateway/go.sum ./
RUN go mod download
COPY packages/gateway/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /gateway ./cmd/gateway

FROM --platform=${TARGETPLATFORM} ${ALPINE_IMAGE}
RUN apk add --no-cache ca-certificates
COPY --from=build /gateway /usr/local/bin/gateway
EXPOSE 8080
ENTRYPOINT ["gateway"]
