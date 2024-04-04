#!/bin/bash
set -e

clog() {
	echo -e "$@" | tee -a "$LOG"
}

silent() {
	echo -e "\t-> $@" >> "$LOG"
	"$@" 2>&1 >> "$LOG"
}

ask() {
	echo -n "$1 (y/n) > " | tee -a "$LOG"
	read choice
	echo "$choice" >> "$LOG"

	case "$choice" in
	  y|Y ) return 0;;
	  n|N ) return 1;;
	  * ) return 1;;
	esac
}

if [ "$EUID" -ne 0 ]; then
	clog "Please run as root"
	exit 1
fi

STARTTIME="$(date +%H:%M:%S)"
LOG="deploy-$(date +%Y-%m-%d)_$STARTTIME.log"

: > "$LOG"

if ! command -v tailscale &>/dev/null; then
    clog "Could not find tailscale, is it installed?"
    exit 1
fi

if [ ! -f bin/tnetmgr ]; then
    clog "Could not find compiled binary"
    exit 1
fi

clog "Installing tailscale network manager service (tnetmgrd)"

clog ">Moving binary"
silent cp bin/tnetmgr /usr/local/bin/tnetmgr

clog ">Copying service"
silent cp configs/tnetmgrd.service /etc/systemd/system/tnetmgrd.service

clog ">Copying default config"
silent mkdir -p /etc/tnetmgr
silent cp configs/config.yml /etc/tnetmgr/config.yml

clog ">Finished"

clog "\nConfig is located here: /etc/netmgr/config.yml"
clog "\nEnded at $(date +%H:%M:%S)"
