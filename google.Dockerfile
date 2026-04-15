FROM registry.suse.com/bci/golang:1.26 as builder

# Ensure a portable, static-ish binary
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

# Copy and download dependencies
COPY go.mod go.sum ./
RUN go mod download

ADD . /build
WORKDIR /build
RUN go build ./cmd/scim-schreiber-google/

FROM registry.suse.com/bci/bci-minimal

COPY --from=builder /build/scim-schreiber-google /scim-schreiber-google
CMD ["/scim-schreiber-google"]