FROM ccr.ccs.tencentyun.com/solat/golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/preview-worker ./cmd/preview-worker
RUN CGO_ENABLED=0 go build -o /preview-worker ./cmd/preview-worker

FROM ccr.ccs.tencentyun.com/solat/alpine:3.20
RUN apk add --no-cache poppler-utils ca-certificates
COPY --from=build /preview-worker /usr/local/bin/preview-worker
EXPOSE 8090
ENV HTTP_ADDR=:8090
CMD ["preview-worker"]
