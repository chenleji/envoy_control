#!/usr/bin/env bash

docker run --rm --name proxy -p 9000:9000 -p 10000:10000  --user 1000:1000 \
       -v $(pwd)/baseline.yaml:/etc/envoy/envoy.yaml envoyproxy/envoy