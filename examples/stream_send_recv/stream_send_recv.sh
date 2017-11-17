#!/usr/bin/env bash

MODE=${1:-"s"}

if [ ${MODE} = "-s" ]; then
    printf "Running sender ...\n"
    ./stream_send_recv -s -d 3 -saddr 127.0.0.1 -snodeid 6 -sport 10025 -count 100 -size 5000000
else
    printf "Running receiver ...\n"
    ./stream_send_recv -r -d 3 -raddr 127.0.0.1 -rnodeid 7 -rport 10025
fi