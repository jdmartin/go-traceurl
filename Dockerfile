FROM golang:latest AS build

#Build 5 Sep 2024, take 2

RUN mkdir /
WORKDIR /
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -gcflags "all=-N -l" -tags netgo -o main .

FROM cgr.dev/chainguard/static:latest
COPY --from=build /main /
COPY --from=build /static /static

CMD ["/main"]
