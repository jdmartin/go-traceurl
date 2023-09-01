FROM cgr.dev/chainguard/go:latest as build

#Build 01 Sep 2023

RUN mkdir /
WORKDIR /
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o main .

FROM cgr.dev/chainguard/glibc-dynamic
COPY --from=build /main /
COPY --from=build /static /static

CMD ["/main"]
