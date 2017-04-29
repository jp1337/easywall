#!/usr/bin/env bash

# ---------------------------------------------------
# ---------- Define some useful functions -----------
# ---------------------------------------------------

function sourceFile {
	echo "Sourcing File $1"
	if [ -f $1 ] ; then
		echo "Found file $1. Sourcing it."
		source $1
	else
		abort "File $1 not found. Aborting."
	fi
}

function logStart {
	DateTime=$(date "+%Y/%m/%d %H:%M:%S")
	log " "
	log "------------------------------------------------------------"
	log "---------- EasyWall $1 $DateTime -----------"
	log "------------------------------------------------------------"
	log " "
}

function resetIPTables {
	log "Beginning to reset the firewall rules and opening the firewall for every connection"

	$IPTABLES -P INPUT ACCEPT
	$IPTABLES -P OUTPUT ACCEPT
	$IPTABLES -P FORWARD ACCEPT
	$IPTABLES -F
	$IPTABLES -X
	$IPTABLES -t nat -F
	$IPTABLES -t nat -X
	$IPTABLES -t mangle -F
	$IPTABLES -t mangle -X

	$IP6TABLES -P INPUT ACCEPT
	$IP6TABLES -P OUTPUT ACCEPT
	$IP6TABLES -P FORWARD ACCEPT
	$IP6TABLES -F
	$IP6TABLES -X
	$IP6TABLES -t nat -F
	$IP6TABLES -t nat -X
	$IP6TABLES -t mangle -F
	$IP6TABLES -t mangle -X

	log "The firewall rules for IPV4 and IPV6 have been successfully reset"
}

function log {
	if [ $LOG = true ];
	then
		mkdir -p $LOGDIR
		touch $LOGDIR$LOGFILE
		
		if [ -n "$1" ]; then
			IN="$1"
			DateTime=$(date "+%Y/%m/%d %H:%M:%S")
			echo $DateTime': '$IN >> $LOGDIR$LOGFILE
		else
			while read IN
			do
				DateTime=$(date "+%Y/%m/%d %H:%M:%S")
				echo $DateTime': '$IN >> $LOGDIR$LOGFILE
			done
		fi
	fi
}

function logIptables {
	if [ $LOG = true ];
	then
		mkdir -p $LOGDIR
		touch $LOGDIR$LOGIPV4
		echo "" > $LOGDIR$LOGIPV4

		while read IN
		do
			echo $IN >> $LOGDIR$LOGIPV4
		done
	fi
}

function logIp6tables {
	if [ $LOG = true ];
	then
		mkdir -p $LOGDIR
		touch $LOGDIR$LOGIPV6
		echo "" > $LOGDIR$LOGIPV6

		while read IN
		do
			echo $IN >> $LOGDIR$LOGIPV6
		done
	fi
}

function abort {
	echo "ERROR: "$1 1>&2
	exit 1
}

function AddSlash {
	local STR=$1
	local length=${#STR}
	local last_char=${STR:length-1:1}
	[[ $last_char != "/" ]] && STR="$STR/"; :
	echo $STR
}

function RemoveSlash {
	local STR=$1
	local length=${#STR}
	local last_char=${STR:length-1:1}
	[[ $last_char == "/" ]] && STR=${STR:0:length-1}; :
	echo $STR
}

function is_ipv4 {
    local  ip=$1
    local  stat=1

    if [[ $ip =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
        OIFS=$IFS
        IFS='.'
        ip=($ip)
        IFS=$OIFS
        [[ ${ip[0]} -le 255 && ${ip[1]} -le 255 && ${ip[2]} -le 255 && ${ip[3]} -le 255 ]]
        stat=$?
    fi
    return $stat
}

function is_ipv6 {
    local  ip=$1
    if [[ $ip =~ ^$|^[0-9a-fA-F]{1,4}:[0-9a-fA-F]{1,4}:[0-9a-fA-F]{1,4}:[0-9a-fA-F]{1,4}:[0-9a-fA-F]{1,4}:[0-9a-fA-F]{1,4}:[0-9a-fA-F]{1,4}:[0-9a-fA-F]{1,4}$ ]]; then
        return 0
    fi
    return 1
}