FROM golang:1.5.3

RUN go get github.com/constabulary/gb/...
RUN mkdir docker-gc
WORKDIR /docker-gc

COPY ./ /docker-gc/

RUN chmod +x bin/docker-gc \
 && script/setup \
 && script/compile
CMD ["script/run"]
