#!/bin/bash

set -x

token=`curl -s "127.0.0.1:8000/login?username=alice&password=alice123&device=iphonex" | jq -r .Token`

curl -s "127.0.0.1:8000/category/2003/11/23?token=$token" | python -m json.tool
curl -s "127.0.0.1:8000/category/2003/11/1?token=$token" | python -m json.tool
curl -s "127.0.0.1:8000/category/2003/11?token=$token" | python -m json.tool
curl -s "127.0.0.1:8000/category/2003/1?token=$token" | python -m json.tool
curl -s "127.0.0.1:8000/category/2003?token=$token" | python -m json.tool
curl -s "127.0.0.1:8000/category/2004?token=$token" | python -m json.tool
curl -s "127.0.0.1:8000/category/2013?token=$token" | python -m json.tool
curl -s "127.0.0.1:8000/category/2018?token=$token" | python -m json.tool
curl -s "127.0.0.1:8000/category?token=$token" | python -m json.tool

pushd /tmp/alice/Photos/master

ls -l 2003/11/23 2003/11/1 2003/1/17 2004/1/8 2013/1/21 2018/7/28
cd ../preview
ls -l 2003/11/23 2003/11/1 2003/1/17 2004/1/8 2013/1/21 2018/7/28

popd
