FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/miniploy ./cmd/miniploy

FROM alpine:3.22
RUN apk add --no-cache git openssh-client docker-cli docker-cli-compose ca-certificates
COPY --from=build /out/miniploy /usr/local/bin/miniploy
ENTRYPOINT ["/usr/local/bin/miniploy"]
