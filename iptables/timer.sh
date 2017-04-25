#!/usr/bin/env bash

# ---------------------------------------------------
# ------- Read in Configuration and Functions -------
# ---------------------------------------------------

# filepath to configuration parameter file
CONFIG=../config/easywall.cfg

source "functions.sh"

readConfig $CONFIG

logStart "timer.sh"

# ---------------------------------------------------
# -------------- Sleep for 30 seconds ---------------
# ---------------------------------------------------

log "timer.sh now starts to sleep 30 seconds"

sleep 30

log "The 30 seconds are over"

# ----------------------------------------------------
# ------------------ Reset IPTABLES ------------------
# ----------------------------------------------------

if [ $APPLIED = false ];
then
	log "The rules were not accepted by the user."
	resetIPTables
else
	log "The rules have been successfully accepted and are not reset"
fi

exit 0