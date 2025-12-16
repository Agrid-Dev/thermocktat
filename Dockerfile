# ---- build stage ----
FROM golang:1.25-alpine AS builder

WORKDIR /src
RUN apk add --no-cache ca-certificates git

# Cache deps
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .

# Build metadata (optional)
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
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
