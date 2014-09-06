#!/bin/sh

set -e

SCANIMAGE="/opt/autoscan/bin/scanimage-wrap"
DAEMON=/opt/autoscan/bin/autoscan
PIDFILE=/var/run/autoscan.pid
USER=thompa
GROUP=scanner
DAEMON_ARGS="-scanimage=$SCANIMAGE -templates=/opt/autoscan/templates -static=/opt/autoscan/static -listen=:8080 -config=/opt/autoscan/etc/autoscan.conf -logfile=/opt/autoscan/log/autoscan.log"

case "$1" in
    start)
	echo heartbeat > /sys/class/leds/led0/trigger
	start-stop-daemon --chdir / --background --make-pidfile --start --pidfile "$PIDFILE" --chuid="$USER:$GROUP" --exec "$DAEMON" -- $DAEMON_ARGS
	;;
    stop)
	start-stop-daemon --stop --quiet --pidfile "$PIDFILE" --exec "$DAEMON"
	;;
    *)
	echo "Invalid"
	exit 1
	;;
esac
