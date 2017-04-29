#!/usr/bin/env bash

# ---------------------------------------------------
# -------- Source Configuration and Functions -------
# ---------------------------------------------------

if [ -f "../iptables/functions.sh" ] ; then
	source "../iptables/functions.sh"
else
	echo "functions.sh not found. Aborting." 1>&2
	exit 1
fi

sourceFile "../config/easywall.cfg"
logStart "apply.sh"


# ----------------------------------------------------
# ------------------ Reset IPTABLES ------------------
# ----------------------------------------------------

resetIPTables


# ---------------------------------------------------
# --------- Setting General chain policies ----------
# ---------------------------------------------------
 
log "Setting General chain policies"

$IPTABLES -P INPUT DROP
$IPTABLES -P OUTPUT ACCEPT

if [ "$IPV6" = "true" ]; then
	log "IPv6 Enabled"
	$IP6TABLES -P INPUT DROP
	$IP6TABLES -P OUTPUT ACCEPT
	echo 0 > /proc/sys/net/ipv6/conf/all/disable_ipv6
	echo 0 > /proc/sys/net/ipv6/conf/default/disable_ipv6
else
	log "IPv6 Disabled"
	$IP6TABLES -P INPUT DROP
	$IP6TABLES -P OUTPUT DROP
	echo 1 > /proc/sys/net/ipv6/conf/all/disable_ipv6
    echo 1 > /proc/sys/net/ipv6/conf/default/disable_ipv6
fi


# ---------------------------------------------------
# --------------- Traffic Forwarding ----------------
# ---------------------------------------------------

if [ "$IPV4FORWARDING" = "true" ] ; then
	log "enable IPv4 forwarding ( ! overwrites sysctl settings ! )"
	$IPTABLES -P FORWARD ACCEPT
	echo 1 > /proc/sys/net/ipv4/ip_forward
else
	log "disable IPv4 forwarding ( ! overwrites sysctl settings ! )"
	$IPTABLES -P FORWARD DROP
	echo 0 > /proc/sys/net/ipv4/ip_forward
	echo 0 > /proc/sys/net/ipv4/conf/all/forwarding
	echo 0 > /proc/sys/net/ipv4/conf/default/forwarding
fi

if [ "$IPV6" = "true" ]; then
	if [ "$IPV6FORWARDING" = "true" ] ; then
		log "enable IPv6 forwarding ( ! overwrites sysctl settings ! )"
		$IP6TABLES -P FORWARD ACCEPT
	else
		log "disable IPv6 forwarding ( ! overwrites sysctl settings ! )"
		$IP6TABLES -P FORWARD DROP
		echo 0 > /proc/sys/net/ipv6/conf/all/forwarding
		echo 0 > /proc/sys/net/ipv6/conf/default/forwarding
	fi
fi


# ---------------------------------------------------
# -------------- Allow Loopback access --------------
# ---------------------------------------------------

log "Allow Loopback access"

$IPTABLES -A INPUT -i lo -j ACCEPT
if [ "$IPV6" = "true" ]; then
	$IP6TABLES -A INPUT -i lo -j ACCEPT
fi


# ---------------------------------------------------
# ------------------ Custom Chains ------------------
# ---------------------------------------------------

# Chain for preventing SSH brute-force attacks.
# Permits 10 new connections within 5 minutes from a single host then drops 
# incomming connections from that host. Beyond a burst of 100 connections we log at up 1 attempt per second to prevent filling of logs.
if [ "$SSHBRUTE" = "true" ]; then
	log "SSH Bruteforce Protection Enabled."
	$IPTABLES -N SSHBRUTE
	$IPTABLES -A SSHBRUTE -m recent --name SSH --set
	$IPTABLES -A SSHBRUTE -m recent --name SSH --update --seconds 300 --hitcount 10 -m limit --limit 1/second --limit-burst 100 -j LOG --log-prefix " easywall[SSH-brute]: "
	$IPTABLES -A SSHBRUTE -m recent --name SSH --update --seconds 300 --hitcount 10 -j DROP
	$IPTABLES -A SSHBRUTE -j ACCEPT
	if [ "$IPV6" = "true" ]; then
		$IP6TABLES -N SSHBRUTE
		$IP6TABLES -A SSHBRUTE -m recent --name SSH --set
		$IP6TABLES -A SSHBRUTE -m recent --name SSH --update --seconds 300 --hitcount 10 -m limit --limit 1/second --limit-burst 100 -j LOG --log-prefix " easywall[SSH-brute]: "
		$IP6TABLES -A SSHBRUTE -m recent --name SSH --update --seconds 300 --hitcount 10 -j DROP
		$IP6TABLES -A SSHBRUTE -j ACCEPT
	fi
fi

# Chain for preventing ping flooding - up to 6 pings per second from a single 
# source, again with log limiting. Also prevents us from ICMP REPLY flooding some victim when replying to ICMP ECHO from a spoofed source.
if [ "$ICMPFLOOD" = "true" ]; then
	log "ICMP Flood Protection Enabled."
	$IPTABLES -N ICMPFLOOD
	$IPTABLES -A ICMPFLOOD -m recent --set --name ICMP --rsource
	$IPTABLES -A ICMPFLOOD -m recent --update --seconds 1 --hitcount 6 --name ICMP --rsource --rttl -m limit --limit 1/sec --limit-burst 1 -j LOG --log-prefix " easywall[ICMP-flood]: "
	$IPTABLES -A ICMPFLOOD -m recent --update --seconds 1 --hitcount 6 --name ICMP --rsource --rttl -j DROP
	$IPTABLES -A ICMPFLOOD -j ACCEPT
	if [ "$IPV6" = "true" ]; then
		$IP6TABLES -N ICMPFLOOD
		$IP6TABLES -A ICMPFLOOD -m recent --set --name ICMP --rsource
		$IP6TABLES -A ICMPFLOOD -m recent --update --seconds 1 --hitcount 6 --name ICMP --rsource --rttl -m limit --limit 1/sec --limit-burst 1 -j LOG --log-prefix " easywall[ICMP-flood]: "
		$IP6TABLES -A ICMPFLOOD -m recent --update --seconds 1 --hitcount 6 --name ICMP --rsource --rttl -j DROP
		$IP6TABLES -A ICMPFLOOD -j ACCEPT
	fi
fi

# invalid drop
if [ "$INVALIDDROP" = "true" ]; then
	log "Dropping invalid packages."
	$IPTABLES -N INVALIDDROP
	$IPTABLES -A INVALIDDROP -m state --state INVALID -m limit --limit 60/m -j LOG --log-prefix " easywall[invalid]: "
	$IPTABLES -A INVALIDDROP -m state --state INVALID -j DROP
	if [ "$IPV6" = "true" ]; then
		$IP6TABLES -N INVALIDDROP
		$IP6TABLES -A INVALIDDROP -m state --state INVALID -m limit --limit 60/m -j LOG --log-prefix " easywall[invalid]: "
		$IP6TABLES -A INVALIDDROP -m state --state INVALID -j DROP
	fi
fi

# portscan drop
if [ "$PORTSCAN" = "true" ]; then
	log "Portscan Detection Enabled"
	$IPTABLES -N PORTSCAN
	$IPTABLES -A PORTSCAN -m limit --limit 60/m -j LOG --log-prefix " easywall[portscan]: "
	$IPTABLES -A PORTSCAN -j DROP
	if [ "$IPV6" = "true" ]; then
		$IP6TABLES -N PORTSCAN
		$IP6TABLES -A PORTSCAN -m limit --limit 60/m -j LOG --log-prefix " easywall[portscan]: "
		$IP6TABLES -A PORTSCAN -j DROP
	fi
fi


# ---------------------------------------------------
# --------------------- Defaults --------------------
# ---------------------------------------------------

# allow established or related connections
$IPTABLES -A INPUT -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
if [ "$IPV6" = "true" ]; then
	$IP6TABLES -A INPUT -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
fi

# Block remote packets claiming to be from a loopback address.
$IPTABLES -A INPUT -s 127.0.0.0/8 ! -i lo -j DROP
if [ "$IPV6" = "true" ]; then
	$IP6TABLES -A INPUT -s ::1/128 ! -i lo -j DROP
fi


# ---------------------------------------------------
# -------- Block IP-addresses from blacklist --------
# ---------------------------------------------------

log "Blocking Blacklisted IP-Addresses"

if [ -f $BLACKLIST ];
then
	$IPTABLES -N BLACKLIST
	
	if [ "$IPV6" = "true" ]; then
		$IP6TABLES -N BLACKLIST
	fi
	
	for ip in $(egrep -v -E "^#|^$" $BLACKLIST); do
		if is_ipv4 $ip ; then
			log "Blocking IPV4 Address $ip"
			$IPTABLES -A BLACKLIST -s $ip -j LOG --log-prefix " easywall[blacklist]: "
			$IPTABLES -A BLACKLIST -s $ip -j DROP
		fi
		if [ "$IPV6" = "true" ]; then
			if is_ipv6 $ip ; then
				log "Blocking IPV6 Address $ip"
				$IP6TABLES -A BLACKLIST -s $ip -j LOG --log-prefix " easywall[blacklist]: "
				$IP6TABLES -A BLACKLIST -s $ip -j DROP
			fi
		fi
	done
	
	$IPTABLES -I INPUT -j BLACKLIST
	
	if [ "$IPV6" = "true" ]; then
		$IP6TABLES -I INPUT -j BLACKLIST
	fi
else
	log "No Blacklist file found! Please fix that. Script has searched here: $BLACKLIST"
fi


# ---------------------------------------------------
# -------- Allow IP-addresses from whitelist --------
# ---------------------------------------------------

log "Allowing all good IP-Addresses"

if [ -f $WHITELIST ];
then
	$IPTABLES -N WHITELIST
	
	if [ "$IPV6" = "true" ]; then
		$IP6TABLES -N WHITELIST
	fi
	
	for ip in $(egrep -v -E "^#|^$" $WHITELIST); do
		if is_ipv4 $ip ; then
			log "Allowing IPV4 Address $ip"
			$IPTABLES -A WHITELIST -s $ip -j ACCEPT
		fi
		if [ "$IPV6" = "true" ]; then
			if is_ipv6 $ip ; then
				log "Allowing IPV6 Address $ip"
				$IP6TABLES -A WHITELIST -s $ip -j ACCEPT
			fi
		fi
	done
	
	$IPTABLES -I INPUT -j WHITELIST
	
	if [ "$IPV6" = "true" ]; then
		$IP6TABLES -I INPUT -j WHITELIST
	fi
else
	log "No Whitelist file found! Please fix that. Script has searched here: $WHITELIST"
fi


# ---------------------------------------------------
# ----------------- Allow TCP Ports -----------------
# ---------------------------------------------------

if [ -f $TCP ];
then
	for port in $(egrep -v -E "^#|^$" $TCP); do
		if [[ $port == *":"* ]]; then
			$IPTABLES -A INPUT -p tcp --match multiport --dports $port --syn -m conntrack --ctstate NEW -j ACCEPT
			if [ "$IPV6" = "true" ]; then
				$IP6TABLES -A INPUT -p tcp --match multiport --dports $port -m conntrack --ctstate NEW -j ACCEPT
			fi
		else
			if [[ $port == *"ssh"* ]]; then
				port=$(echo $port| cut -d';' -f 1)
				$IPTABLES -A INPUT -p tcp --dport $port --syn -m conntrack --ctstate NEW -j SSHBRUTE
				if [ "$IPV6" = "true" ]; then
					$IP6TABLES -A INPUT -p tcp --dport $port -m conntrack --ctstate NEW -j SSHBRUTE
				fi
			else
				$IPTABLES -A INPUT -p tcp --dport $port --syn -m conntrack --ctstate NEW -j ACCEPT
				if [ "$IPV6" = "true" ]; then
					$IP6TABLES -A INPUT -p tcp --dport $port -m conntrack --ctstate NEW -j ACCEPT
				fi
			fi
		fi
	

	done
else
	echo "No TCP Port-List found!"
fi


# ---------------------------------------------------
# ----------------- Allow UDP Ports -----------------
# ---------------------------------------------------

if [ -f $UDP ];
then
	for port in $(egrep -v -E "^#|^$" $UDP); do
		if [[ $port == *":"* ]]; then
			$IPTABLES -A INPUT -p udp --match multiport --dports $port -m conntrack --ctstate NEW -j ACCEPT
			if [ "$IPV6" = "true" ]; then
				$IP6TABLES -A INPUT -p udp --match multiport --dports $port -m conntrack --ctstate NEW -j ACCEPT
			fi
		else
			if [[ $port == *"ssh"* ]]; then
				port=$(echo $port| cut -d';' -f 1)
				$IPTABLES -A INPUT -p udp --dport $port -m conntrack --ctstate NEW -j SSHBRUTE
				if [ "$IPV6" = "true" ]; then
					$IP6TABLES -A INPUT -p udp --dport $port -m conntrack --ctstate NEW -j SSHBRUTE
				fi
			else
				$IPTABLES -A INPUT -p udp --dport $port -m conntrack --ctstate NEW -j ACCEPT
				if [ "$IPV6" = "true" ]; then
					$IP6TABLES -A INPUT -p udp --dport $port -m conntrack --ctstate NEW -j ACCEPT
				fi
			fi
		fi
	done
else
	echo "No TCP Port-List found!"
fi


# ---------------------------------------------------
# ------ Restrict ICMP to useful packet types -------
# ---------------------------------------------------

if [ "$ICMP" = "true" ]; then
	log "Filtering ICMP Packets to only useful"
	
	# Permit useful IMCP packet types for IPv4
	# Note: RFC 792 states that all hosts MUST respond to ICMP ECHO requests.
	$IPTABLES -A INPUT -p icmp --icmp-type 0  -m conntrack --ctstate NEW -j ACCEPT
	$IPTABLES -A INPUT -p icmp --icmp-type 3  -m conntrack --ctstate NEW -j ACCEPT
	$IPTABLES -A INPUT -p icmp --icmp-type 11 -m conntrack --ctstate NEW -j ACCEPT

	# Permit needed ICMP packet types for IPv6 per RFC 4890.
	if [ "$IPV6" = "true" ]; then
		$IP6TABLES -A INPUT              -p ipv6-icmp --icmpv6-type 1   -j ACCEPT
		$IP6TABLES -A INPUT              -p ipv6-icmp --icmpv6-type 2   -j ACCEPT
		$IP6TABLES -A INPUT              -p ipv6-icmp --icmpv6-type 3   -j ACCEPT
		$IP6TABLES -A INPUT              -p ipv6-icmp --icmpv6-type 4   -j ACCEPT
		$IP6TABLES -A INPUT              -p ipv6-icmp --icmpv6-type 128 -j ACCEPT
		$IP6TABLES -A INPUT              -p ipv6-icmp --icmpv6-type 129 -j ACCEPT
		$IP6TABLES -A INPUT              -p ipv6-icmp --icmpv6-type 144 -j ACCEPT
		$IP6TABLES -A INPUT              -p ipv6-icmp --icmpv6-type 145 -j ACCEPT
		$IP6TABLES -A INPUT              -p ipv6-icmp --icmpv6-type 146 -j ACCEPT
		$IP6TABLES -A INPUT              -p ipv6-icmp --icmpv6-type 147 -j ACCEPT
	fi
else
	log "Dropping all ICMP Packets. This is not recommended!"
	
	$IPTABLES -A INPUT -p icmp -m conntrack --ctstate NEW -j DROP
	$IP6TABLES -A INPUT -p ipv6-icmp -j DROP
fi


# ---------------------------------------------------
# -------------- ICMP Flood Prevention --------------
# ---------------------------------------------------

# Permit IMCP echo requests (ping) and use ICMPFLOOD chain for preventing ping flooding.
if [ "$ICMPFLOOD" = "true" ]; then
	$IPTABLES -A INPUT -p icmp --icmp-type 8  -m conntrack --ctstate NEW -j ICMPFLOOD
	if [ "$IPV6" = "true" ]; then
		$IP6TABLES -A INPUT -p ipv6-icmp --icmpv6-type 128 -j ICMPFLOOD
	fi
fi


# ---------------------------------------------------
# ---------- Drop and Log invalid packages ----------
# ---------------------------------------------------

if [ "$INVALIDDROP" = "true" ]; then
	$IPTABLES -A INPUT -m state --state INVALID -j INVALIDDROP
	if [ "$IPV6" = "true" ]; then
		$IP6TABLES -A INPUT -m state --state INVALID -j INVALIDDROP
	fi
fi


# ---------------------------------------------------
# --------- Broadcast, Multicast and Anycast --------
# ---------------------------------------------------

# Broadcast
if [ "$BROADCAST" = "true" ]; then
	$IPTABLES -A INPUT -m addrtype --dst-type BROADCAST -j DROP
fi

# Multicast
if [ "$MULTICAST" = "true" ]; then
	$IPTABLES -A INPUT -m addrtype --dst-type MULTICAST -j DROP
	$IPTABLES -A INPUT -d 224.0.0.0/4 -j DROP
	if [ "$IPV6" = "true" ]; then
		$IP6TABLES -A INPUT -m addrtype --dst-type MULTICAST -j DROP
	fi
fi

# Anycast
if [ "$ANYCAST" = "true" ]; then
	$IPTABLES -A INPUT -m addrtype --dst-type ANYCAST -j DROP
	if [ "$IPV6" = "true" ]; then
		$IP6TABLES -A INPUT -m addrtype --dst-type ANYCAST -j DROP
	fi
fi


# ---------------------------------------------------
# ---------------- Portscan Detection ---------------
# ---------------------------------------------------

if [ "$PORTSCAN" = "true" ]; then
	## nmap Null scans / no flags
	$IPTABLES -A INPUT -p tcp --tcp-flags ALL NONE -j PORTSCAN
	## nmap FIN stealth scan
	$IPTABLES -A INPUT -p tcp --tcp-flags ALL FIN -j PORTSCAN
	## SYN + FIN
	$IPTABLES -A INPUT -p tcp --tcp-flags SYN,FIN SYN,FIN -j PORTSCAN
	## SYN + RST
	$IPTABLES -A INPUT -p tcp --tcp-flags SYN,RST SYN,RST -j PORTSCAN
	## FIN + RST
	$IPTABLES -A INPUT -p tcp --tcp-flags FIN,RST FIN,RST -j PORTSCAN
	## FIN + URG + PSH
	$IPTABLES -A INPUT -p tcp --tcp-flags ALL FIN,URG,PSH -j PORTSCAN
	## XMAS
	$IPTABLES -A INPUT -p tcp --tcp-flags ALL URG,ACK,PSH,RST,SYN,FIN -j PORTSCAN
	## ALL
	$IPTABLES -A INPUT -p tcp --tcp-flags ALL ALL -j PORTSCAN
	## FIN/PSH/URG without ACK
	$IPTABLES -A INPUT -p tcp --tcp-flags ACK,FIN FIN -j PORTSCAN
	$IPTABLES -A INPUT -p tcp --tcp-flags ACK,PSH PSH -j PORTSCAN
	$IPTABLES -A INPUT -p tcp --tcp-flags ACK,URG URG -j PORTSCAN
	if [ "$IPV6" = "true" ]; then
		## nmap Null scans / no flags
		$IP6TABLES -A INPUT -p tcp --tcp-flags ALL NONE -j PORTSCAN
		## nmap FIN stealth scan
		$IP6TABLES -A INPUT -p tcp --tcp-flags ALL FIN -j PORTSCAN
		## SYN + FIN
		$IP6TABLES -A INPUT -p tcp --tcp-flags SYN,FIN SYN,FIN -j PORTSCAN
		## SYN + RST
		$IP6TABLES -A INPUT -p tcp --tcp-flags SYN,RST SYN,RST -j PORTSCAN
		## FIN + RST
		$IP6TABLES -A INPUT -p tcp --tcp-flags FIN,RST FIN,RST -j PORTSCAN
		## FIN + URG + PSH
		$IP6TABLES -A INPUT -p tcp --tcp-flags ALL FIN,URG,PSH -j PORTSCAN
		## XMAS
		$IP6TABLES -A INPUT -p tcp --tcp-flags ALL URG,ACK,PSH,RST,SYN,FIN -j PORTSCAN
		## ALL
		$IP6TABLES -A INPUT -p tcp --tcp-flags ALL ALL -j PORTSCAN
		## FIN/PSH/URG without ACK
		$IP6TABLES -A INPUT -p tcp --tcp-flags ACK,FIN FIN -j PORTSCAN
		$IP6TABLES -A INPUT -p tcp --tcp-flags ACK,PSH PSH -j PORTSCAN
		$IP6TABLES -A INPUT -p tcp --tcp-flags ACK,URG URG -j PORTSCAN
	fi
fi


# ---------------------------------------------------
# ------------------- Auth Packets ------------------
# ---------------------------------------------------

# Good practise is to explicately reject AUTH traffic so that it fails fast.
if [ "$DROPAUTH" = "true" ]; then
	log "Auth Packages will be dropped fast"
	$IPTABLES -A INPUT -p tcp --dport 113 --syn -m conntrack --ctstate NEW -j REJECT --reject-with tcp-reset
	if [ "$IPV6" = "true" ]; then
		$IP6TABLES -A INPUT -p tcp --dport 113 -m conntrack --ctstate NEW -j REJECT --reject-with tcp-reset
	fi
else
	log "Allowing Auth Packages"
	$IPTABLES -A INPUT -p tcp --dport 113 --syn -m conntrack --ctstate NEW -j ACCEPT
	if [ "$IPV6" = "true" ]; then
		$IP6TABLES -A INPUT -p tcp --dport 113 -m conntrack --ctstate NEW -j ACCEPT
	fi
fi


# ---------------------------------------------------
# ------------------- Final Rules -------------------
# ---------------------------------------------------

# log all other packages and reject them
$IPTABLES -A INPUT -j LOG
$IPTABLES -A INPUT -j REJECT
if [ "$IPV6" = "true" ]; then
	$IP6TABLES -A INPUT -j LOG
	$IP6TABLES -A INPUT -j REJECT
fi


# ----------------------------------------------------
# ------------------ Save New Rules ------------------
# ----------------------------------------------------

log "Saving new iptables rules"
$IPTABLES_SAVE | logIptables

if [ "$IPV6" = "true" ]; then
	log "Saving new ip6tables rules"
	$IP6TABLES_SAVE | logIp6tables
fi

exit 0