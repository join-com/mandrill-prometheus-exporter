### [stage 1]
FROM golang:1.15-alpine as prebuild

RUN mkdir -p /go/src/mandrill-prometheus-exporter
WORKDIR /go/src/mandrill-prometheus-exporter

RUN apk add --update git \
    && rm -rf /var/cache/apk/*

COPY . /go/src/mandrill-prometheus-exporter
RUN go get -v && go build


### [stage 2]
FROM alpine:3.7

COPY --from=prebuild /go/bin/mandrill-prometheus-exporter /usr/local/bin/
EXPOSE 9153
ENTRYPOINT ["mandrill-prometheus-exporter"]
