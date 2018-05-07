# Start with a Golang-enabled base image.
FROM golang:1.10.0 as builder

# Fetch all dependencies.
RUN go get github.com/coredns/coredns
RUN go get github.com/opentracing/opentracing-go
RUN go get github.com/sirupsen/logrus
RUN go get github.com/mitchellh/hashstructure
RUN go get k8s.io/client-go/...
RUN rm -rf /go/src/github.com/coredns/coredns/vendor/github.com/golang/glog

# Mount the central and edge plugins.
COPY plugin/edge /go/src/github.com/optikon/coredns/plugin/edge

# Mount the custom plugin.cfg file.
COPY plugin/plugin.cfg /go/src/github.com/coredns/coredns/plugin.cfg

# Build the custom CoreDNS binary.
WORKDIR /go/src/github.com/coredns/coredns
RUN make

# Build a runtime container to use the custom binary.
FROM alpine:latest
MAINTAINER Ross Flieger-Allison

# Only need ca-certificates & openssl if want to use DNS over TLS (RFC 7858).
RUN apk --no-cache add bind-tools ca-certificates openssl && update-ca-certificates

# Copy the custom binary from the build container.
COPY --from=builder /go/src/github.com/coredns/coredns/coredns /coredns

# Expose DNS ports.
EXPOSE 53 53/udp

# Expose the daemon process port.
EXPOSE 8053

# Mount the executable for entry.
ENTRYPOINT ["/coredns"]
