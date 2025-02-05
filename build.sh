#!/bin/bash

PLATFORMS=("linux/amd64")

for platform in "${PLATFORMS[@]}"
do
    split=(${platform//\// })
    GOOS=${split[0]} 
    GOARCH=${split[1]}
    CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build -o build/smc-exporter-${GOOS}-${GOARCH}
done
