#!/usr/bin/env bash
export OS_CLOUD=catalystcloud
rm -rf clouds.yaml
cp clouds.yml clouds.yaml
go run src/os-mfa.go
