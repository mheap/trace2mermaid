# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /build

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.version=dev -X main.commit=local -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -trimpath \
    -o trace2mermaid .

# Runtime stage
FROM alpine:3.21

WORKDIR /

COPY --from=builder /build/trace2mermaid /usr/local/bin/trace2mermaid

RUN adduser -D -u 1000 appuser
USER appuser

ENTRYPOINT ["/usr/local/bin/trace2mermaid"]
