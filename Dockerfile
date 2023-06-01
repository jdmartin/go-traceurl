FROM golang:1.20.4-alpine3.18 as build

#Build 1 June 2023

RUN mkdir /
WORKDIR /
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o main .


FROM alpine:3.18.0
COPY --from=build /main /
COPY --from=build /static /static

CMD ["/main"]
