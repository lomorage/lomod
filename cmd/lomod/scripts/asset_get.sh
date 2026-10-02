#!/bin/bash

set -x

token=`curl -s "127.0.0.1:8000/login?username=alice&password=alice123&device=iphonex" | jq -r .Token`

curl -s -o 2.jpg "127.0.0.1:8000/asset/2.jpg?token=$token"
open 2.jpg
curl -s -o 3.jpg "127.0.0.1:8000/asset/3.jpg?token=$token"
open 3.jpg

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

curl -s -o 8.tar "127.0.0.1:8000/asset/8.liph?token=$token"
tar -xf 8.tar
open 8.jpg
open 8.mov

curl -s -o 8_preview.jpg "127.0.0.1:8000/preview/8.liph?token=$token"
open 8_preview.jpg
curl -s -o 8_preview_320x240.jpg "127.0.0.1:8000/preview/8.liph?token=$token&width=320&height=240"
open 8_preview_320x240.jpg

curl -s -o 9.heic "127.0.0.1:8000/asset/9.heic?token=$token"
open 9.heic
curl -s -o 9_preview.jpg "127.0.0.1:8000/preview/9.heic?token=$token&width=75&height=75"
open 9_preview.jpg
