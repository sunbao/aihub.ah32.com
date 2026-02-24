# syntax=docker/dockerfile:1

FROM golang:1.24 AS build
WORKDIR /src

ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn
ENV GOPROXY=$GOPROXY
ENV GOSUMDB=$GOSUMDB

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/aihub-api ./cmd/api
RUN CGO_ENABLED=0 go build -trimpath -o /out/aihub-worker ./cmd/worker
RUN CGO_ENABLED=0 go build -trimpath -o /out/aihub-migrate ./cmd/migrate

FROM alpine:3.19
ARG ALPINE_REPO_BASE=https://dl-cdn.alpinelinux.org/alpine
RUN sed -i "s|https://dl-cdn.alpinelinux.org/alpine|${ALPINE_REPO_BASE}|g" /etc/apk/repositories \
  && apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=build /out/aihub-api /usr/local/bin/aihub-api
COPY --from=build /out/aihub-worker /usr/local/bin/aihub-worker
COPY --from=build /out/aihub-migrate /usr/local/bin/aihub-migrate
COPY migrations /app/migrations
COPY docker/entrypoint.sh /entrypoint.sh

RUN chmod +x /entrypoint.sh

EXPOSE 8080
ENTRYPOINT ["/entrypoint.sh"]
