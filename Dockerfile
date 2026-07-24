# Build a static, CGO-free webpcli binary and ship it on distroless/static.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO disabled: the codec is pure Go, so the binary is fully static.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/webpcli ./cmd/webpcli

FROM gcr.io/distroless/static-debian12 AS runtime
COPY --from=build /out/webpcli /webpcli
ENTRYPOINT ["/webpcli"]
