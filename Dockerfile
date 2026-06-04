FROM golang:1.25-alpine AS build
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /storage-api ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ffmpeg ca-certificates
COPY --from=build /storage-api /usr/local/bin/storage-api
EXPOSE 8080
CMD ["storage-api"]
