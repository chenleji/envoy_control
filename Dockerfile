FROM golang:1.10 as builder
MAINTAINER chenleji@gmail.com

COPY ./ /go/src/github.com/chenleji/envoy_control/
WORKDIR /go/src/github.com/chenleji/envoy_control/
RUN go build -o envoy_control

FROM centos:7
MAINTAINER chenleji@gmail.com
RUN mkdir -p /conf && \
    cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime
WORKDIR /
COPY --from=builder /go/src/github.com/chenleji/envoy_control/envoy_control .

CMD /envoy_control