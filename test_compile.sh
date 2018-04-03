#!/bin/bash

set -e
pwd
go get github.com/OpenBazaar/multiwallet
go get github.com/mattn/go-sqlite3
go test -coverprofile=multiwallet.cover.out ./
echo "mode: set" > coverage.out && cat *.cover.out | grep -v mode: | sort -r | \
awk '{if($1 != last) {print $0;last=$1}}' >> coverage.out
rm -rf *.cover.out