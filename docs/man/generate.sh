#!/bin/bash
#
# Convert pod files to manual pages, using pod2man.
# 
# Assumes files are named like:
#   <name>.<section>.pod

set -e

for IN in *.pod; do
	OUT=$( basename $IN .pod )
	SECTION=${OUT##*.}
	NAME=${OUT%.*}

	podchecker $IN
	pod2man --section=$SECTION --name=$NAME \
		--release "" --center "" \
		$IN $OUT
done
