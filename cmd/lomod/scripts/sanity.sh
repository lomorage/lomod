#!/bin/bash

set -x

./user_create.sh

read -p "Start import assets. Press any key to continue... " -n1 -s

./asset_import_sha.sh

token=`curl -s "127.0.0.1:8000/login?username=alice&password=alice123&device=iphonex" | jq -r .Token`

read -p "Upload same content should get conflict. Press any key to continue... " -n1 -s
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/img/true_2003_11_23.jpg  "127.0.0.1:8000/asset?token=$token&ext=jpg&createtime=2003-11-23T12:00:00Z" | python -m json.tool

./asset_list.sh

read -p "List assets by day, month, year, and all. After review above metadata, press any key to continue get asset... " -n1 -s
./asset_get.sh

read -p "Test jitt preview. Press any key to continue... " -n1 -s
rm -rf /tmp/alice/Photos/preview/*

token=`curl -s "127.0.0.1:8000/login?username=alice&password=alice123&device=iphonex" | jq -r .Token`

curl -s -o 1_preview.jpg "127.0.0.1:8000/preview/1.jpg?token=$token"
open 1_preview.jpg
curl -s -o 1_preview_320x240.jpg "127.0.0.1:8000/preview/1.jpg?token=$token&width=320&height=240"
open 1_preview_320x240.jpg

curl -s -o 6_preview.jpg "127.0.0.1:8000/preview/6.mp4?token=$token"
open 6_preview.jpg
curl -s -o 6_preview_320x240.jpg "127.0.0.1:8000/preview/6.mp4?token=$token&width=320&height=240"
open 6_preview_320x240.jpg

curl -s -o 7_preview.png "127.0.0.1:8000/preview/7.png?token=$token"
open 7_preview.png
curl -s -o 7_preview_320x240.png "127.0.0.1:8000/preview/7.png?token=$token&width=320&height=240"
open 7_preview_320x240.png

curl -s -o 8_preview.jpg "127.0.0.1:8000/preview/8.liph?token=$token"
open 8_preview.jpg
curl -s -o 8_preview_320x240.jpg "127.0.0.1:8000/preview/8.liph?token=$token&width=320&height=240"
open 8_preview_320x240.jpg

read -p "Test Delete. Press any key to continue... " -n1 -s
./asset_delete.sh
./asset_list.sh
read -p "List assets again, press any key to continue... " -n1 -s
