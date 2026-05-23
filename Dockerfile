FROM golang:1.26-alpine AS builder

WORKDIR /build

COPY . .

RUN go build -o dasredis .

FROM alpine:3.21

COPY --from=builder /build/dasredis /usr/local/bin/dasredis

EXPOSE 6380

ENTRYPOINT ["dasredis"]
