FROM golang:1.5

RUN go get github.com/constabulary/gb/...
RUN mkdir docker-gc
WORKDIR /docker-gc

COPY ./ /docker-gc/

RUN script/setup \
 && script/compile \
 && chmod +x bin/docker-gc
CMD bin/bin/docker-gc -command=ttl -interval=5m -images_ttl=5m -containers_ttl=1m 

