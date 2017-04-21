#!/bin/bash

# ---------------------------------------------------
# ---------- Define some useful functions -----------
# ---------------------------------------------------

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
	
	#echo $DateTime': '$IN
fi
}

function abort {
	echo "ERROR: "$1 1>&2
	exit 1
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


# ---------------------------------------------------
# ---------- Read Configuration Parameters ----------
# ---------------------------------------------------

# filepath to configuration parameter file
CONFIG=../config/easywall.cfg

echo "Reading Config File $CONFIG"
if [ -f $CONFIG ];
then
	echo "Found config file. Reading lines"
	source $CONFIG
else
	abort "Config File not found."
fi


# ----------------------------------------------------
# ------ Save Existing Rules and Reset IPTABLES ------
# ----------------------------------------------------

# Save existings rules in log file
log "Saving IPTables Configuration"
$IPTABLES_SAVE | log
log "Saving IP6Tables Configuration"
$IP6TABLES_SAVE | log

# Delete all existing rules in all chains for starting with a clean firewall
log "Clearing IPTables Rules."
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
log "All Rules are cleared."


# ---------------------------------------------------
# ---------- Add General Connection Rules -----------
# ---------------------------------------------------
 
# general rules for all connections
$IPTABLES -P INPUT DROP
$IPTABLES -P OUTPUT ACCEPT
$IPTABLES -P FORWARD DROP
$IP6TABLES -P INPUT DROP
$IP6TABLES -P OUTPUT ACCEPT
$IP6TABLES -P FORWARD DROP

# allow localhost input and output
$IPTABLES -A INPUT -i lo -j ACCEPT
$IP6TABLES -A INPUT -i lo -j ACCEPT



# ---------------------------------------------------
# ------ Blacklist and Whitelist IP-Addresses -------
# ---------------------------------------------------

# blacklist
if [ -f $BLACKLIST ];
then
	$IPTABLES -N BLACKLIST
	$IP6TABLES -N BLACKLIST
	for ip in $(egrep -v -E "^#|^$" $BLACKLIST); do
		if is_ipv4 $ip ; then
			$IPTABLES -A BLACKLIST -s $ip -j LOG --log-prefix "BLACKLIST"
			$IPTABLES -A BLACKLIST -s $ip -j DROP
		fi
		if is_ipv6 $ip ; then
			$IP6TABLES -A BLACKLIST -s $ip -j LOG --log-prefix "BLACKLIST"
			$IP6TABLES -A BLACKLIST -s $ip -j DROP
		fi
	done
	$IPTABLES -I INPUT -j BLACKLIST
	$IP6TABLES -I INPUT -j BLACKLIST
else
	echo "No Blacklist found!"
fi

# whitelist
if [ -f $WHITELIST ];
then
	$IPTABLES -N WHITELIST
	$IP6TABLES -N WHITELIST
	for ip in $(egrep -v -E "^#|^$" $WHITELIST); do
		if is_ipv4 $ip ; then
			$IPTABLES -A WHITELIST -s $ip -j LOG --log-prefix "WHITELIST"
			$IPTABLES -A WHITELIST -s $ip -j ACCEPT
		fi
		if is_ipv6 $ip ; then
			$IP6TABLES -A WHITELIST -s $ip -j LOG --log-prefix "WHITELIST"
			$IP6TABLES -A WHITELIST -s $ip -j ACCEPT
		fi
	done
	$IPTABLES -I INPUT -j WHITELIST
	$IP6TABLES -I INPUT -j WHITELIST
else
	echo "No Whitelist found!"
fi


# ---------------------------------------------------
# ------------- Add Some Security Rules -------------
# ---------------------------------------------------

# allow established or related connections
$IPTABLES -A INPUT -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
$IP6TABLES -A INPUT -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT

# drop all invalid packages
$IPTABLES -A INPUT -m conntrack --ctstate INVALID -j DROP
$IP6TABLES -A INPUT -m conntrack --ctstate INVALID -j DROP

# Block remote packets claiming to be from a loopback address.
$IPTABLES -A INPUT -s 127.0.0.0/8 ! -i lo -j DROP
$IP6TABLES -A INPUT -s ::1/128 ! -i lo -j DROP

# Drop all packets that are going to broadcast, multicast or anycast address.
$IPTABLES -A INPUT -m addrtype --dst-type BROADCAST -j DROP
$IPTABLES -A INPUT -m addrtype --dst-type MULTICAST -j DROP
$IP6TABLES -A INPUT -m addrtype --dst-type MULTICAST -j DROP
$IPTABLES -A INPUT -m addrtype --dst-type ANYCAST -j DROP
$IP6TABLES -A INPUT -m addrtype --dst-type ANYCAST -j DROP
$IPTABLES -A INPUT -d 224.0.0.0/4 -j DROP

# Chain for preventing SSH brute-force attacks.
# Permits 10 new connections within 5 minutes from a single host then drops 
# incomming connections from that host. Beyond a burst of 100 connections we log at up 1 attempt per second to prevent filling of logs.
$IPTABLES -N SSHBRUTE
$IPTABLES -A SSHBRUTE -m recent --name SSH --set
$IPTABLES -A SSHBRUTE -m recent --name SSH --update --seconds 300 --hitcount 10 -m limit --limit 1/second --limit-burst 100 -j LOG --log-prefix "iptables[SSH-brute]: "
$IPTABLES -A SSHBRUTE -m recent --name SSH --update --seconds 300 --hitcount 10 -j DROP
$IPTABLES -A SSHBRUTE -j ACCEPT
$IP6TABLES -N SSHBRUTE
$IP6TABLES -A SSHBRUTE -m recent --name SSH --set
$IP6TABLES -A SSHBRUTE -m recent --name SSH --update --seconds 300 --hitcount 10 -m limit --limit 1/second --limit-burst 100 -j LOG --log-prefix "iptables[SSH-brute]: "
$IP6TABLES -A SSHBRUTE -m recent --name SSH --update --seconds 300 --hitcount 10 -j DROP
$IP6TABLES -A SSHBRUTE -j ACCEPT

# Chain for preventing ping flooding - up to 6 pings per second from a single 
# source, again with log limiting. Also prevents us from ICMP REPLY flooding some victim when replying to ICMP ECHO from a spoofed source.
$IPTABLES -N ICMPFLOOD
$IPTABLES -A ICMPFLOOD -m recent --set --name ICMP --rsource
$IPTABLES -A ICMPFLOOD -m recent --update --seconds 1 --hitcount 6 --name ICMP --rsource --rttl -m limit --limit 1/sec --limit-burst 1 -j LOG --log-prefix "iptables[ICMP-flood]: "
$IPTABLES -A ICMPFLOOD -m recent --update --seconds 1 --hitcount 6 --name ICMP --rsource --rttl -j DROP
$IPTABLES -A ICMPFLOOD -j ACCEPT
$IP6TABLES -N ICMPFLOOD
$IP6TABLES -A ICMPFLOOD -m recent --set --name ICMP --rsource
$IP6TABLES -A ICMPFLOOD -m recent --update --seconds 1 --hitcount 6 --name ICMP --rsource --rttl -m limit --limit 1/sec --limit-burst 1 -j LOG --log-prefix "iptables[ICMP-flood]: "
$IP6TABLES -A ICMPFLOOD -m recent --update --seconds 1 --hitcount 6 --name ICMP --rsource --rttl -j DROP
$IP6TABLES -A ICMPFLOOD -j ACCEPT



# ---------------------------------------------------
# ------------ Add own TCP and UDP Rules ------------
# ---------------------------------------------------

# tcp ports
if [ -f $TCP ];
then
	for port in $(egrep -v -E "^#|^$" $TCP); do
		if [[ $port == *":"* ]]; then
			$IPTABLES -A INPUT -p tcp --match multiport --dports $port --syn -m conntrack --ctstate NEW -j SSHBRUTE
			$IP6TABLES -A INPUT -p tcp --match multiport --dports $port -m conntrack --ctstate NEW -j SSHBRUTE
		else
			if [[ $port == *"ssh"* ]]; then
				port=$(echo $port| cut -d';' -f 1)
				$IPTABLES -A INPUT -p tcp --dport $port --syn -m conntrack --ctstate NEW -j SSHBRUTE
				$IP6TABLES -A INPUT -p tcp --dport $port -m conntrack --ctstate NEW -j SSHBRUTE
			else
				$IPTABLES -A INPUT -p tcp --dport $port --syn -m conntrack --ctstate NEW -j ACCEPT
				$IP6TABLES -A INPUT -p tcp --dport $port -m conntrack --ctstate NEW -j ACCEPT
			fi
		fi
	

	done
else
	echo "No TCP Port-List found!"
fi


# udp ports
if [ -f $UDP ];
then
	for port in $(egrep -v -E "^#|^$" $UDP); do
		if [[ $port == *":"* ]]; then
			$IPTABLES -A INPUT -p udp --match multiport --dports $port -m conntrack --ctstate NEW -j SSHBRUTE
			$IP6TABLES -A INPUT -p udp --match multiport --dports $port -m conntrack --ctstate NEW -j SSHBRUTE
		else
			if [[ $port == *"ssh"* ]]; then
				port=$(echo $port| cut -d';' -f 1)
				$IPTABLES -A INPUT -p udp --dport $port -m conntrack --ctstate NEW -j SSHBRUTE
				$IP6TABLES -A INPUT -p udp --dport $port -m conntrack --ctstate NEW -j SSHBRUTE
			else
				$IPTABLES -A INPUT -p udp --dport $port -m conntrack --ctstate NEW -j ACCEPT
				$IP6TABLES -A INPUT -p udp --dport $port -m conntrack --ctstate NEW -j ACCEPT
			fi
		fi
	done
else
	echo "No TCP Port-List found!"
fi



# ---------------------------------------------------
# ------------- ICMP and Security Rules -------------
# ---------------------------------------------------


# Permit useful IMCP packet types for IPv4
# Note: RFC 792 states that all hosts MUST respond to ICMP ECHO requests.
# Blocking these can make diagnosing of even simple faults much more tricky.
# Real security lies in locking down and hardening all services, not by hiding.
$IPTABLES -A INPUT -p icmp --icmp-type 0  -m conntrack --ctstate NEW -j ACCEPT
$IPTABLES -A INPUT -p icmp --icmp-type 3  -m conntrack --ctstate NEW -j ACCEPT
$IPTABLES -A INPUT -p icmp --icmp-type 11 -m conntrack --ctstate NEW -j ACCEPT

# Permit needed ICMP packet types for IPv6 per RFC 4890.
$IP6TABLES -A INPUT              -p ipv6-icmp --icmpv6-type 1   -j ACCEPT
$IP6TABLES -A INPUT              -p ipv6-icmp --icmpv6-type 2   -j ACCEPT
$IP6TABLES -A INPUT              -p ipv6-icmp --icmpv6-type 3   -j ACCEPT
$IP6TABLES -A INPUT              -p ipv6-icmp --icmpv6-type 4   -j ACCEPT
$IP6TABLES -A INPUT              -p ipv6-icmp --icmpv6-type 133 -j ACCEPT
$IP6TABLES -A INPUT              -p ipv6-icmp --icmpv6-type 134 -j ACCEPT
$IP6TABLES -A INPUT              -p ipv6-icmp --icmpv6-type 135 -j ACCEPT
$IP6TABLES -A INPUT              -p ipv6-icmp --icmpv6-type 136 -j ACCEPT
$IP6TABLES -A INPUT              -p ipv6-icmp --icmpv6-type 137 -j ACCEPT
$IP6TABLES -A INPUT              -p ipv6-icmp --icmpv6-type 141 -j ACCEPT
$IP6TABLES -A INPUT              -p ipv6-icmp --icmpv6-type 142 -j ACCEPT
$IP6TABLES -A INPUT -s fe80::/10 -p ipv6-icmp --icmpv6-type 130 -j ACCEPT
$IP6TABLES -A INPUT -s fe80::/10 -p ipv6-icmp --icmpv6-type 131 -j ACCEPT
$IP6TABLES -A INPUT -s fe80::/10 -p ipv6-icmp --icmpv6-type 132 -j ACCEPT
$IP6TABLES -A INPUT -s fe80::/10 -p ipv6-icmp --icmpv6-type 143 -j ACCEPT
$IP6TABLES -A INPUT              -p ipv6-icmp --icmpv6-type 148 -j ACCEPT
$IP6TABLES -A INPUT              -p ipv6-icmp --icmpv6-type 149 -j ACCEPT
$IP6TABLES -A INPUT -s fe80::/10 -p ipv6-icmp --icmpv6-type 151 -j ACCEPT
$IP6TABLES -A INPUT -s fe80::/10 -p ipv6-icmp --icmpv6-type 152 -j ACCEPT
$IP6TABLES -A INPUT -s fe80::/10 -p ipv6-icmp --icmpv6-type 153 -j ACCEPT

# Permit IMCP echo requests (ping) and use ICMPFLOOD chain for preventing ping 
# flooding.
$IPTABLES -A INPUT -p icmp --icmp-type 8  -m conntrack --ctstate NEW -j ICMPFLOOD
$IP6TABLES -A INPUT -p ipv6-icmp --icmpv6-type 128 -j ICMPFLOOD

# Do not log packets that are going to ports used by SMB
# (Samba / Windows Sharing).
$IPTABLES -A INPUT -p udp -m multiport --dports 135,445 -j DROP
$IPTABLES -A INPUT -p udp --dport 137:139 -j DROP
$IPTABLES -A INPUT -p udp --sport 137 --dport 1024:65535 -j DROP
$IPTABLES -A INPUT -p tcp -m multiport --dports 135,139,445 -j DROP
$IP6TABLES -A INPUT -p udp -m multiport --dports 135,445 -j DROP
$IP6TABLES -A INPUT -p udp --dport 137:139 -j DROP
$IP6TABLES -A INPUT -p udp --sport 137 --dport 1024:65535 -j DROP
$IP6TABLES -A INPUT -p tcp -m multiport --dports 135,139,445 -j DROP

# Do not log packets that are going to port used by UPnP protocol.
$IPTABLES -A INPUT -p udp --dport 1900 -j DROP
$IP6TABLES -A INPUT -p udp --dport 1900 -j DROP

# Do not log late replies from nameservers.
$IPTABLES -A INPUT -p udp --sport 53 -j DROP
$IP6TABLES -A INPUT -p udp --sport 53 -j DROP

# Good practise is to explicately reject AUTH traffic so that it fails fast.
$IPTABLES -A INPUT -p tcp --dport 113 --syn -m conntrack --ctstate NEW -j REJECT --reject-with tcp-reset
$IP6TABLES -A INPUT -p tcp --dport 113 -m conntrack --ctstate NEW -j REJECT --reject-with tcp-reset

# Prevent DOS by filling log files.
# $IPTABLES -A INPUT -m limit --limit 1/second --limit-burst 100 -j LOG --log-prefix "iptables[DOS]: "
# $IP6TABLES -A INPUT -m limit --limit 1/second --limit-burst 100 -j LOG --log-prefix "iptables[DOS]: "

# on tcp connections only allow syn packets. this will filter all other packets which are not SYN.
$IPTABLES -A INPUT -p tcp ! --syn -m state --state NEW -m limit --limit 5/m --limit-burst 7 -j LOG --log-level 4 --log-prefix "SYN FILTER"
$IP6TABLES -A INPUT -p tcp -m state --state NEW -m limit --limit 5/m --limit-burst 7 -j LOG --log-level 4 --log-prefix "SYN FILTER"
$IPTABLES -A INPUT -p tcp ! --syn -m state --state NEW -j DROP
$IP6TABLES -A INPUT -p tcp -m state --state NEW -j DROP

# force fragments packets ckeck.
$IPTABLES -A INPUT -f -m limit --limit 5/m --limit-burst 7 -j LOG --log-level 4 --log-prefix "FRAGMENTS FILTER"
$IPTABLES -A INPUT -f -j DROP

# drop all xmas packages
$IPTABLES  -A INPUT -p tcp --tcp-flags ALL FIN,URG,PSH -j DROP
$IP6TABLES  -A INPUT -p tcp --tcp-flags ALL FIN,URG,PSH -j DROP
$IPTABLES  -A INPUT -p tcp --tcp-flags ALL ALL -j DROP
$IP6TABLES  -A INPUT -p tcp --tcp-flags ALL ALL -j DROP

# drop all null packets
$IPTABLES  -A INPUT -p tcp --tcp-flags ALL NONE -m limit --limit 5/m --limit-burst 7 -j LOG --log-level 4 --log-prefix "NULL Packets"
$IP6TABLES  -A INPUT -p tcp --tcp-flags ALL NONE -m limit --limit 5/m --limit-burst 7 -j LOG --log-level 4 --log-prefix "NULL Packets"
$IPTABLES  -A INPUT -p tcp --tcp-flags ALL NONE -j DROP
$IP6TABLES  -A INPUT -p tcp --tcp-flags ALL NONE -j DROP

# block other bad stuff
$IPTABLES  -A INPUT -p tcp --tcp-flags SYN,RST SYN,RST -j DROP
$IP6TABLES  -A INPUT -p tcp --tcp-flags SYN,RST SYN,RST -j DROP

$IPTABLES  -A INPUT -p tcp --tcp-flags SYN,FIN SYN,FIN -m limit --limit 5/m --limit-burst 7 -j LOG --log-level 4 --log-prefix "XMAS Packets"
$IP6TABLES  -A INPUT -p tcp --tcp-flags SYN,FIN SYN,FIN -m limit --limit 5/m --limit-burst 7 -j LOG --log-level 4 --log-prefix "XMAS Packets"
$IPTABLES  -A INPUT -p tcp --tcp-flags SYN,FIN SYN,FIN -j DROP #XMAS
$IP6TABLES  -A INPUT -p tcp --tcp-flags SYN,FIN SYN,FIN -j DROP #XMAS
 
$IPTABLES  -A INPUT -p tcp --tcp-flags FIN,ACK FIN -m limit --limit 5/m --limit-burst 7 -j LOG --log-level 4 --log-prefix "Fin Packets Scan"
$IP6TABLES  -A INPUT -p tcp --tcp-flags FIN,ACK FIN -m limit --limit 5/m --limit-burst 7 -j LOG --log-level 4 --log-prefix "Fin Packets Scan"
$IPTABLES  -A INPUT -p tcp --tcp-flags FIN,ACK FIN -j DROP # FIN packet scans
$IP6TABLES  -A INPUT -p tcp --tcp-flags FIN,ACK FIN -j DROP # FIN packet scans
 
$IPTABLES  -A INPUT -p tcp --tcp-flags ALL SYN,RST,ACK,FIN,URG -j DROP
$IP6TABLES  -A INPUT -p tcp --tcp-flags ALL SYN,RST,ACK,FIN,URG -j DROP

# log everything else
$IPTABLES -A INPUT -j LOG
$IP6TABLES -A INPUT -j LOG

exit 0
