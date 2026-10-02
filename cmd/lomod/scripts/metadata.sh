#!/bin/bash

set -x

token=`curl -s "127.0.0.1:8000/login?username=alice&password=alice123&device=iphonex" | jq -r .Token`

curl -s -X POST -H 'content-type:application/json' --data-binary @./metadata.json  "127.0.0.1:8000/assets/metadata?token=$token"
curl -s "127.0.0.1:8000/assets/metadata/category?token=$token" | python -m json.tool
curl -s "127.0.0.1:8000/assets/metadata/geo/names?token=$token" | python -m json.tool
curl -s "127.0.0.1:8000/assets/metadata/face/names?token=$token" | python -m json.tool
curl -s "127.0.0.1:8000/assets/metadata/geo/city/values?token=$token" | python -m json.tool

curl -s "127.0.0.1:8000/assets/metadata?token=$token&source-device=ios&miss-category=geo" | python -m json.tool
curl -s "127.0.0.1:8000/assets/metadata?token=$token&source-device=ios&category=geo&ver-less=2" | python -m json.tool
curl -s "127.0.0.1:8000/assets/metadata?token=$token&source-device=ios&category=geo&miss-name=city" | python -m json.tool
curl -s "127.0.0.1:8000/assets/metadata?token=$token&source-device=ios&category=geo&name=city&ver-less=2" | python -m json.tool

curl -s "127.0.0.1:8000/assets?token=$token&meta-nv=city,san%20jose" | python -m json.tool
