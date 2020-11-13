#!/bin/bash
pkill -9 '^(harmony|soldier|commander|profiler|bootnode)$' | sed 's/^/Killed process: /'
rm -rf db-*
# sudo tc qdisc del dev lo root netem delay 100ms
sudo tc qdisc del dev ens5 root netem delay 100ms
sudo tc qdisc del dev enp125s0 root netem delay 100ms
