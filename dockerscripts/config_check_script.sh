#!/bin/sh
CONFIG_FILE=/root/.minio/config.json

[ -f $CONFIG_FILE ] && echo "Config File is already exists" || (mkdir -p /root/.minio && cp /tmp/config.json /root/.minio/config.json && echo "Config File is copied")

rm /tmp/config.json
