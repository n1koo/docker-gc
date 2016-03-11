FROM golang:1.6-alpine

RUN apk update && apk add bash git perl

ADD . /go/src/app
WORKDIR /go/src/app

RUN ./script/setup
RUN ./script/compile

CMD /go/src/app/bin/docker-gc
