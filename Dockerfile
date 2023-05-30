FROM golang:1.20.4-alpine3.18

RUN mkdir /app

ADD . /app

WORKDIR /app

RUN go build -o main .

CMD ["/app/main"]
