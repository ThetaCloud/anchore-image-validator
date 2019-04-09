FROM golang:1.12-alpine AS builder

RUN apk add --update --no-cache ca-certificates make git curl mercurial

ARG PACKAGE=github.com/banzaicloud/anchore-image-validator

RUN mkdir -p /build
WORKDIR /build

COPY go.* /build/
RUN go mod download
COPY . /build
RUN go install ./cmd 

FROM alpine:3.9

COPY --from=builder /go/bin/cmd /usr/local/bin/anchore-image-validator
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

USER 65534:65534

ENTRYPOINT ["/usr/local/bin/anchore-image-validator"]
