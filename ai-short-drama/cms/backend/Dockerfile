FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/cms-api ./cmd/server

FROM alpine:3.22

RUN apk add --no-cache ca-certificates docker-cli tzdata

WORKDIR /app

COPY --from=builder /out/cms-api /usr/local/bin/cms-api

EXPOSE 8888

ENTRYPOINT ["cms-api"]
