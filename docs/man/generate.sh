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

	# If it has not changed in git, set the mtime to the last commit that
	# touched the file.
	CHANGED=$( git status --porcelain -- "$IN" | wc -l )
	if [ $CHANGED -eq 0 ]; then
		GIT_MTIME=$( git log --pretty=%at -n1 -- "$IN" )
		touch -d "@$GIT_MTIME" "$IN"
	fi

	podchecker $IN
	pod2man --section=$SECTION --name=$NAME \
		--release "" --center "" \
		$IN $OUT
done
