#!/bin/bash

set -x

curl -X POST -d '{"Name":"alice", "Password": "alice123", "Phone": "4084088888", "Email": "alice@lomorage.io", "NickName": "Alice"}' -H "Content-Type: application/json" "127.0.0.1:8002/account"
token=`curl -s -X POST "127.0.0.1:8002/login?username=alice&password=alice123&device=iphonex" | jq -r .Token`
