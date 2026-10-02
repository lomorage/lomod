#!/bin/bash

set -x

token=`curl -s "127.0.0.1:8000/login?username=alice&password=alice123&device=iphonex" | jq -r .Token`

curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/img/true_2003_11_23.jpg  "127.0.0.1:8000/asset?token=$token&ext=jpg&createtime=2003-11-23T12:00:00Z" | python -m json.tool
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/img/true_2003_11_01_1.jpg  "127.0.0.1:8000/asset?token=$token&ext=jpg&createtime=2003-11-01T16:00:00Z" | python -m json.tool
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/img/true_2003_11_01_2.jpg  "127.0.0.1:8000/asset?token=$token&ext=jpg&createtime=2003-11-01T16:00:00Z" | python -m json.tool
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/img/true_2003_01_17.jpg  "127.0.0.1:8000/asset?token=$token&ext=jpg&createtime=2003-01-17T20:00:00Z" | python -m json.tool
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/img/true_2004_1_21.jpg  "127.0.0.1:8000/asset?token=$token&ext=jpg&createtime=2013-01-21T09:55:50Z" | python -m json.tool
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/video/true_2013_08_08.mp4  "127.0.0.1:8000/asset?token=$token&ext=mp4&createtime=2013-08-08T08:08:08Z" | python -m json.tool
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/img/true_2018_07_28.png  "127.0.0.1:8000/asset?token=$token&ext=png&createtime=2013-07-28T08:08:08Z" | python -m json.tool

cp ../test/img/true_2003_01_17.jpg 8.jpg; cp ../test/video/true_2013_08_08.mp4 8.mov; zip 8.zip 8.jpg 8.mov
dd bs=1000 count=1 if=8.zip of=8.zip.part1
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @./8.zip.part1 "127.0.0.1:8000/asset?token=$token&ext=zip&createtime=2013-11-23T12:00:00Z" | python -m json.tool
dd skip=1000 bs=1 count=79603 if=8.zip of=8.zip.part2
curl -s -X PATCH -H 'If-Match: size=1000, sha1=e950e850fcef58b7b339866b7ca01a976ad88bc7' -H 'content-type:application/octet-stream' --data-binary @./8.zip.part2 "127.0.0.1:8000/asset?token=$token&ext=zip&createtime=2013-11-23T12:00:00Z" | python -m json.tool

curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/img/img_4479.heic  "127.0.0.1:8000/asset?token=$token&ext=heif&createtime=2013-11-23T08:08:08Z" | python -m json.tool

cp ../test/video/true_2013_08_08.mp4 10.mov
cp ../test/img/img_4479.heic 10.heic; zip 10.zip 10.heic 10.mov
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @./10.zip "127.0.0.1:8000/asset?token=$token&ext=zip&createtime=2013-11-23T12:00:00Z" | python -m json.tool
