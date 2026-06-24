FROM registry.suse.com/bci/golang:1.26 AS builder

# Ensure a portable, static-ish binary
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

# Copy and download dependencies
COPY go.mod go.sum ./
RUN go mod download

ADD . /build
WORKDIR /build
RUN go build ./cmd/scim-schreiber-ldap/

FROM registry.suse.com/bci/bci-base:16.0 AS cert-builder

RUN zypper addrepo -C 'https://download.opensuse.org/repositories/SUSE:/CA/$releasever/' SUSE_CA
RUN zypper --gpg-auto-import-keys refresh

RUN zypper install -y ca-certificates-suse

FROM registry.suse.com/bci/bci-minimal:16.0
COPY --from=cert-builder /var/lib/ca-certificates/ca-bundle.pem /var/lib/ca-certificates/ca-bundle.pem
COPY --from=builder /build/scim-schreiber-ldap /scim-schreiber-ldap
CMD ["/scim-schreiber-ldap"]