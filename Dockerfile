FROM cgr.dev/chainguard/go:latest as build

#Build 18 Dec 2023

RUN mkdir /
WORKDIR /
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -gcflags "all=-N -l" -tags netgo -o main .

FROM cgr.dev/chainguard/glibc-dynamic
COPY --from=build /main /
COPY --from=build /static /static

CMD ["/main"]
