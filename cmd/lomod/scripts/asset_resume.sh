#!/bin/bash

set -x

token=`curl -s "127.0.0.1:8000/login?username=alice&password=alice123&device=iphonex" | jq -r .Token`

curl -v -X GET "127.0.0.1:8000/metadata/4ebf54db04f335ff66bfc1fd982be62bf23fc967?token=$token"

curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/img/true_2003_11_23.jpg.part1  "127.0.0.1:8000/asset/4ebf54db04f335ff66bfc1fd982be62bf23fc967?token=$token&ext=jpg&createtime=2003-11-23T12:00:00Z" | python -m json.tool

curl -v -X GET "127.0.0.1:8000/metadata/4ebf54db04f335ff66bfc1fd982be62bf23fc967?token=$token"

curl -s -X PATCH -H 'If-Match: size=1000, sha1=631ab5ab5befe28f88ad5c2af28e5def4b477a67' -H 'content-type:application/octet-stream' --data-binary @../test/img/true_2003_11_23.jpg.part2 "127.0.0.1:8000/asset/4ebf54db04f335ff66bfc1fd982be62bf23fc967?token=$token&ext=jpg&createtime=2003-11-23T12:00:00Z" | python -m json.tool

curl -s -X GET "127.0.0.1:8000/metadata/4ebf54db04f335ff66bfc1fd982be62bf23fc967?token=$token"

curl -s -X DELETE "127.0.0.1:8000/asset/4ebf54db04f335ff66bfc1fd982be62bf23fc967?token=$token&byhash=1"

curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/img/true_2003_11_23.jpg.part1  "127.0.0.1:8000/asset/4ebf54db04f335ff66bfc1fd982be62bf23fc967?token=$token&ext=jpg&createtime=2003-11-23T12:00:00Z" | python -m json.tool
curl -s -X GET "127.0.0.1:8000/metadata/4ebf54db04f335ff66bfc1fd982be62bf23fc967?token=$token"

curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/img/true_2003_11_23.jpg  "127.0.0.1:8000/asset/4ebf54db04f335ff66bfc1fd982be62bf23fc967?token=$token&ext=jpg&createtime=2003-11-23T12:00:00Z" | python -m json.tool
curl -s -X GET "127.0.0.1:8000/metadata/4ebf54db04f335ff66bfc1fd982be62bf23fc967?token=$token"


