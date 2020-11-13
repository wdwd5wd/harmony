#!/usr/bin/env bash

# ./test/kill_node.sh
rm -rf tmp_log*
rm *.rlp
rm -rf .dht*
./net_release.sh
scripts/go_executable_build.sh -S || exit 1  # dynamic builds are faster for debug iteration...
./test/bootnode-deploy.sh -B -D 600000 ./test/configs/multi-resharding.txt
