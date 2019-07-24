#!/bin/bash -eu

if [ ! -L "/usr/local/bin/sto" ]; then
	ln -s /go/src/github.com/function61/varasto/rel/sto_linux-amd64 /usr/local/bin/sto
fi

source /build-common.sh

BINARY_NAME="sto"
COMPILE_IN_DIRECTORY="cmd/sto"

# vendor dir contains non-gofmt code..
GOFMT_TARGETS="cmd/ pkg/"

standardBuildProcess
