#!/bin/bash

if [ "$1" == "-release" ]; then
    VERSION=$(cat VERSION)
else
    VERSION="$(cat VERSION)-a.Dev"
fi


PLATFORMS=("linux/amd64")

BUILD_DATE=$(date --iso-8601=m --utc)
GIT_BRANCH=$(git rev-parse --abbrev-ref HEAD 2> /dev/null)

LDFLAGS="-X github.com/prometheus/common/version.BuildUser=$USER
         -X github.com/prometheus/common/version.BuildDate=$BUILD_DATE
         -X github.com/prometheus/common/version.Version=$VERSION 
         -X github.com/prometheus/common/version.Branch=$GIT_BRANCH"

for platform in "${PLATFORMS[@]}"
do
    split=(${platform//\// })
    GOOS=${split[0]} 
    GOARCH=${split[1]}
    OUTPUT_PATH="build/smc-exporter-${VERSION}-${GOOS}-${GOARCH}"
    echo "Building ${OUTPUT_PATH}..."
    CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build -o "${OUTPUT_PATH}" -ldflags "$LDFLAGS"
    chmod +x "${OUTPUT_PATH}"
done
