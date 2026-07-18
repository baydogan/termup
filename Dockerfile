# ---- build stage ----
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO disabled: pure-Go build for now (no sqlite/CGO deps yet)
RUN CGO_ENABLED=0 go build -o /out/ztd ./cmd/ztd

# ---- run stage ----
FROM alpine:3.20
COPY --from=build /out/ztd /usr/local/bin/ztd
ENTRYPOINT ["ztd"]
