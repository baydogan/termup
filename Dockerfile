# ---- build stage ----
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# modernc sqlite is pure Go, so a static (CGO-free) build works.
RUN CGO_ENABLED=0 go build -o /out/termupd ./cmd/termupd \
 && CGO_ENABLED=0 go build -o /out/termup ./cmd/termup

# ---- run stage ----
FROM alpine:3.20
# CA certs are required to verify targets' TLS certificates when probing HTTPS.
RUN apk add --no-cache ca-certificates
COPY --from=build /out/termupd /out/termup /usr/local/bin/
# Working dir holds config.yaml (mounted) and termup.db (persisted volume).
WORKDIR /data
CMD ["termupd"]
