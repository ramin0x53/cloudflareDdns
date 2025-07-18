FROM golang:1.24-alpine AS builder

RUN apk add --no-cache ca-certificates

WORKDIR /app/cloudflareDdns
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o cloudflareDdns

FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

COPY --from=builder /app/cloudflareDdns/cloudflareDdns /cloudflareDdns

ENTRYPOINT ["/cloudflareDdns"]
