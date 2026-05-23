FROM golang:1.26-alpine AS builder

WORKDIR /build

COPY . .

RUN go build -o dasdis .

FROM alpine:3.21

COPY --from=builder /build/dasdis /usr/local/bin/dasdis

EXPOSE 6380

ENTRYPOINT ["dasdis"]
