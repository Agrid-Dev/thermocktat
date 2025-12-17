# ---- build stage ----
FROM golang:1.25-alpine AS builder

WORKDIR /src
RUN apk add --no-cache ca-certificates git

# Cache deps
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .


# BuildKit provides these automatically.
ARG TARGETOS
ARG TARGETARCH
ARG TARGETPLATFORM
ARG BUILDPLATFORM

RUN echo "Building for TARGETPLATFORM=${TARGETPLATFORM:-$BUILDPLATFORM} (TARGETOS=$TARGETOS TARGETARCH=$TARGETARCH)"

# Build metadata (optional)
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

RUN set -eu; \
    os="${TARGETOS:-$(echo ${BUILDPLATFORM:-linux/amd64} | cut -d/ -f1)}"; \
    arch="${TARGETARCH:-$(echo ${BUILDPLATFORM:-linux/amd64} | cut -d/ -f2)}"; \
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -ldflags "-s -w \
    -X github.com/Agrid-Dev/thermocktat/internal/buildinfo.Version=${VERSION} \
    -X github.com/Agrid-Dev/thermocktat/internal/buildinfo.Commit=${COMMIT} \
    -X github.com/Agrid-Dev/thermocktat/internal/buildinfo.Date=${DATE}" \
    -o /out/thermocktat ./cmd/thermocktat

# ---- runtime stage (tiny) ----
FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/thermocktat /thermocktat

EXPOSE 8080
ENTRYPOINT ["/thermocktat"]
