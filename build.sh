#!/bin/bash

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <version>"
    exit 1
fi

VERSION=$1

PLATFORMS=("linux/amd64")

BUILD_DATE=$(date --iso-8601=m --utc)
GIT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
GIT_REVISION=$(git rev-parse HEAD)

LDFLAGS="-X github.com/prometheus/common/version.BuildUser=$USER
         -X github.com/prometheus/common/version.BuildDate=$BUILD_DATE
         -X github.com/prometheus/common/version.Version=$VERSION 
         -X github.com/prometheus/common/version.Branch=$GIT_BRANCH
         -X github.com/prometheus/common/version.Revision=$GIT_REVISION"

for platform in "${PLATFORMS[@]}"
do
    split=(${platform//\// })
    GOOS=${split[0]} 
    GOARCH=${split[1]}
    OUTPUT_PATH="build/smc-exporter-${VERSION}-${GOOS}-${GOARCH}"
    CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build -o "${OUTPUT_PATH}" -ldflags "$LDFLAGS"
    chmod +x "${OUTPUT_PATH}"
done
