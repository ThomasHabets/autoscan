#!/bin/sh

set -e

case "$1" in
    start)
	echo starting...
	echo heartbeat > /sys/class/leds/led0/trigger
	;;
    *)
	echo "Invalid"
	exit 1
	;;
esac
