#!/bin/bash
# Wrapper so chamuyero scripts can invoke chasquid-util for testing.

# Run from the config directory because data_dir is relative.
cd config
go run ../../../cmd/chasquid-util/chasquid-util.go -C=. "$@"
