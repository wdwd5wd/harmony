#!/usr/bin/env bash

# 如果在非bootnode的机器上，不能省略
./test/kill_node.sh
rm -rf tmp_log*
rm *.rlp
rm -rf .dht*
./net_release.sh
scripts/go_executable_build.sh -S || exit 1  # dynamic builds are faster for debug iteration...

# sudo tc qdisc add dev lo root netem delay 100ms
sudo tc qdisc add dev ens5 root netem delay 100ms
sudo tc qdisc add dev enp125s0 root netem delay 100ms

./test/multi-deploy.sh -B -D 600000 ./test/configs/multi-resharding.txt
