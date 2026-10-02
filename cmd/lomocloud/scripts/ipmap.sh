#!/bin/bash

set -x

curl -s -X POST --data-binary @./ipmap.json "127.0.0.1:8002/account/ip_map"

curl -s -X GET "127.0.0.1:8002/account/ip_map"
