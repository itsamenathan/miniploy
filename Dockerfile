FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/miniploy ./cmd/miniploy \
 && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/miniployctl ./cmd/miniployctl

FROM alpine:3.22
RUN apk add --no-cache git openssh-client docker-cli docker-cli-compose docker-cli-buildx ca-certificates
COPY --from=build /out/miniploy /usr/local/bin/miniploy
COPY --from=build /out/miniployctl /usr/local/bin/miniployctl
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD miniployctl health
ENTRYPOINT ["/usr/local/bin/miniploy"]
