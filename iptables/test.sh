#!/usr/bin/env bash

function log {
	if [ $LOG = true ];
	then
		echo "log: true"
	else
		echo "log: false"
	fi
}

LOG=false

log "TEST"
