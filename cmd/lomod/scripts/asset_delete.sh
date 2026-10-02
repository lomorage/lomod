#!/bin/bash

set -x

token=`curl -s "127.0.0.1:8000/login?username=alice&password=alice123&device=iphonex" | jq -r .Token`

curl -s -X DELETE "127.0.0.1:8000/asset/2.jpg?token=$token"
curl -s -X DELETE "127.0.0.1:8000/asset/575db2e474109f982ead09e7f8676680a679c9c0?token=$token&byhash=1"
curl -s -X DELETE -H "Accept: application/json" --data-binary @../test/sample_delete.json "127.0.0.1:8000/asset?token=$token" | python -m json.tool

curl -s -X DELETE "127.0.0.1:8000/asset/6.mp4?token=$token"
curl -s -X DELETE "127.0.0.1:8000/asset/7.png?token=$token"
curl -s -X DELETE "127.0.0.1:8000/asset/8.liph?token=$token"
curl -s -X DELETE "127.0.0.1:8000/asset/9.heic?token=$token"
