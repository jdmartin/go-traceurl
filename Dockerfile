FROM golang:alpine AS build
RUN apk add --no-cache ca-certificates tzdata
RUN mkdir /sockdir

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-w -s" -tags netgo -o /main .

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build --chown=65532:65532 /sockdir /run/go-trace
COPY --from=build /main /main
COPY --from=build /src/static /static
ENV SOCKET_PATH=/run/go-trace/go-trace.sock

USER 65532:65532
ENTRYPOINT ["/main"]
