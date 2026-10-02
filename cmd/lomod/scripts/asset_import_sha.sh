#!/bin/bash

set -x

token=`curl -s "127.0.0.1:8000/login?username=alice&password=alice123&device=iphonex" | jq -r .Token`

curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/img/true_2003_11_23.jpg  "127.0.0.1:8000/asset/4ebf54db04f335ff66bfc1fd982be62bf23fc967?token=$token&ext=jpg&createtime=2003-11-23T12:00:00Z" | python -m json.tool
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/img/true_2003_11_01_1.jpg  "127.0.0.1:8000/asset/7426655f042e8605385cb413de639373b934b18d?token=$token&ext=jpg&createtime=2003-11-01T16:00:00Z" | python -m json.tool
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/img/true_2003_11_01_2.jpg  "127.0.0.1:8000/asset/575db2e474109f982ead09e7f8676680a679c9c0?token=$token&ext=jpg&createtime=2003-11-01T16:00:00Z" | python -m json.tool
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/img/true_2003_01_17.jpg  "127.0.0.1:8000/asset/17363532de7bc73e42823c1448bd52ffe45d4bfc?token=$token&ext=jpg&createtime=2003-01-17T20:00:00Z" | python -m json.tool
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/img/true_2004_1_21.jpg  "127.0.0.1:8000/asset/d4d8773112f68162949b9578f6e476c7f6c8af1f?token=$token&ext=jpg&createtime=2013-01-21T09:55:50Z" | python -m json.tool
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/video/true_2013_08_08.mp4  "127.0.0.1:8000/asset/5d14e3ce82d2101b3b8ece22487d81d824a5745a?token=$token&ext=mp4&createtime=2013-08-08T08:08:08Z" | python -m json.tool
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/img/true_2018_07_28.png  "127.0.0.1:8000/asset/921196cc106f070668dbb2ff3eafc01017540a52?token=$token&ext=png&createtime=2013-07-28T08:08:08Z" | python -m json.tool

cp ../test/img/true_2003_01_17.jpg 8.jpg; cp ../test/video/true_2013_08_08.mp4 8.mov; zip 8.zip 8.jpg 8.mov
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @./8.zip "127.0.0.1:8000/asset/adf6b68f7e71912a4e2666533d4c8619f1b9ddb1?token=$token&ext=zip&createtime=2013-11-23T12:00:00Z" | python -m json.tool

curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/img/img_4479.heic  "127.0.0.1:8000/asset/2a8210982e4cfbeb56d283f43fea9c118a53a839?token=$token&ext=heif&createtime=2013-11-23T08:08:08Z" | python -m json.tool
cp ../test/img/img_4479.heic 10.heic; zip 10.zip 10.heic 8.mov
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @./10.zip "127.0.0.1:8000/asset/7c672f891ac7eae12f8cbd188bf55524dac4242d?token=$token&ext=zip&createtime=2013-11-23T12:00:00Z" | python -m json.tool

# slow motion
curl -s -X POST -H 'content-type:application/octet-stream' --data-binary @../test/video/slow_motion.mov "127.0.0.1:8000/asset/b12fb8a5bb04738efe66f186a45f0388d8fabff1?token=$token&ext=mp4&createtime=2013-08-08T08:08:08Z&filesha=a6902dc1b38136afe949a78d7d458d760125c864" | python -m json.tool
