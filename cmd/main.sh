#!/bin/bash

# shellcheck disable=SC2046
. $(pwd)/.env

export GOSUMDB=off
export GOPRIVATE=github.com
export GO111MODULE=on

go_tidy() {
  rm -rf go.mod go.sum
  touch go.mod
  requirements=$(cat $(pwd)/requirements.txt)

  cat <<EOF > go.mod
module github.com/vngcloud/cloud-provider-vngcloud

go 1.21

require (
 $requirements
)
EOF

  $GO_CMD mod tidy
}

opt=$1
case $opt in
"go-tidy")
  go_tidy
  ;;
esac