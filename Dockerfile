FROM golang:alpine AS build

#Build 17 Sep 2025

WORKDIR /
COPY . .
RUN CGO_ENABLED=0 GOEXPERIMENT=greenteagc go build -ldflags="-w -s" -gcflags "all=-N -l" -tags netgo -o main .

FROM cgr.dev/chainguard/static:latest
COPY --from=build /main /
COPY --from=build /static /static

CMD ["/main"]
