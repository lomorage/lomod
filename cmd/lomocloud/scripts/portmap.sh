#!/bin/bash

set -x

reply=`curl -s -X POST "127.0.0.1:8002/login?username=alice&password=alice123&device=iphonex"`
echo $reply
token=`echo $reply | jq -r .Token`

curl -s -X POST "127.0.0.1:8002/account/portmap/400/4000?token=$token&ip=127.0.0.1"
curl -s -X PUT  "127.0.0.1:8002/account/portmap/400/4000?token=$token&ip=127.0.0.2"
