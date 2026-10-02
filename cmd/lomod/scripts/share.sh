#!/bin/bash

set -x

token=`curl -s "127.0.0.1:8000/login?username=bob&password=bob123&device=iphonex" | jq -r .Token`
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/img/true_2003_11_23.jpg  "127.0.0.1:8000/asset?username=bob&token=$token&ext=jpg&createtime=2003-11-23T12:00:00Z" | python -m json.tool

token=`curl -s "127.0.0.1:8000/login?username=alice&password=alice123&device=iphonex" | jq -r .Token`
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/img/true_2003_11_01_1.jpg  "127.0.0.1:8000/asset?token=$token&ext=jpg&createtime=2003-11-01T16:00:00Z" | python -m json.tool
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/img/true_2003_11_01_2.jpg  "127.0.0.1:8000/asset?token=$token&ext=jpg&createtime=2003-11-01T16:00:00Z" | python -m json.tool
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/img/true_2003_01_17.jpg  "127.0.0.1:8000/asset?token=$token&ext=jpg&createtime=2003-01-17T20:00:00Z" | python -m json.tool
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/img/true_2004_1_21.jpg  "127.0.0.1:8000/asset?token=$token&ext=jpg&createtime=2013-01-21T09:55:50Z" | python -m json.tool
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/video/true_2013_08_08.mp4  "127.0.0.1:8000/asset?token=$token&ext=mp4&createtime=2004-01-08T08:08:08Z" | python -m json.tool

curl -s -X POST  "127.0.0.1:8000/send/user/2/2.jpg?token=$token" | python -m json.tool
curl -s -X POST  "127.0.0.1:8000/send/user/2/3.jpg?token=$token" | python -m json.tool
curl -s -X POST  "127.0.0.1:8000/send/user/4/2.jpg?token=$token" | python -m json.tool
curl -s -X POST  "127.0.0.1:8000/send/group/1/2.jpg?token=$token" | python -m json.tool
curl -s -X POST  "127.0.0.1:8000/send/group/2/2.jpg?token=$token" | python -m json.tool

token=`curl -s "127.0.0.1:8000/login?username=bob&password=bob123&device=iphonex" | jq -r .Token`
curl -s -X POST  "127.0.0.1:8000/send/user/1/1.jpg?username=bob&token=$token" | python -m json.tool
curl -s -X POST  "127.0.0.1:8000/send/group/1/1.jpg?username=bob&token=$token" | python -m json.tool
