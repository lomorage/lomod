#!/bin/bash

set -x

token=`curl -s "127.0.0.1:8000/login?username=alice&password=alice123&device=iphonex" | jq -r .Token`

curl -s -X POST  "127.0.0.1:8000/group/family?token=$token" | python -m json.tool
curl -s -X POST  "127.0.0.1:8000/group/1/2?token=$token"
curl -s -X POST  "127.0.0.1:8000/group/1/3?token=$token"
curl -s -X POST  "127.0.0.1:8000/group/favorite?token=$token" | python -m json.tool
curl -s -X POST  "127.0.0.1:8000/group/2/4?token=$token"
