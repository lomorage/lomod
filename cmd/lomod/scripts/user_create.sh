#!/bin/bash

set -x

# hash(alice123) = 246172676f6e32696424763d3139246d3d343039362c743d332c703d312459577870593256416247397462334a685a3255756247397462336468636d55244f53412f796e7a744e4a59533267676564775647785369783461414a666638784b774c694571647866755500
#curl -X POST -d '{"Name":"alice", "Password": "246172676f6e32696424763d3139246d3d343039362c743d332c703d312459577870593256416247397462334a685a3255756247397462336468636d55244f53412f796e7a744e4a59533267676564775647785369783461414a666638784b774c694571647866755500", "Phone": "4084088888", "Email": "alice@lomorage.io", "NickName": "Alice", "HomeDir": "/tmp/alice"}' -H "Content-Type: application/json" "127.0.0.1:8000/user"

curl -X POST -d '{"Name":"alice", "Password": "alice123", "Phone": "4084088888", "Email": "alice@lomorage.io", "NickName": "Alice", "HomeDir": "/tmp/alice"}' -H "Content-Type: application/json" "127.0.0.1:8000/user"
curl -X POST -d '{"Name":"bob", "Password": "bob123", "Phone": "4084088888", "Email": "bob@lomorage.io", "NickName": "Bob", "HomeDir": "/tmp/bob"}' -H "Content-Type: application/json" "127.0.0.1:8000/user"
token=`curl -s "127.0.0.1:8000/login?username=alice&password=alice123&device=iphonex" | jq -r .Token`

# login from 2nd device
token2=`curl -s "127.0.0.1:8000/login?username=alice&password=alice123&device=ipad" | jq -r .Token`

# login1's token should be good
curl -X POST -d '{"Name":"bob", "Password": "bob123", "Phone": "4084088888", "Email": "bob@lomorage.io", "NickName": "Bob", "HomeDir": "/tmp/bob"}' -H "Content-Type: application/json" "127.0.0.1:8000/user?token=$token"

# login2's token should be good
curl -X POST -d '{"Name":"charlie", "Password": "charlie123", "Phone": "4084088888", "Email": "charlie@lomorage.io", "NickName": "Charlie", "HomeDir": "/tmp/charlie"}' -H "Content-Type: application/json" "127.0.0.1:8000/user?token=$token2"
curl -X POST -d '{"Name":"denny", "Password": "denny123", "Phone": "4084088888", "Email": "denny@lomorage.io", "NickName": "Denny", "HomeDir": "/tmp/denny"}' -H "Content-Type: application/json" "127.0.0.1:8000/user?token=$token2"

# log in other users
curl -s "127.0.0.1:8000/login?username=bob&password=bob123&device=ipad"
curl -s "127.0.0.1:8000/login?username=charlie&password=charlie123&device=ipad"
curl -s "127.0.0.1:8000/login?username=denny&password=denny123&device=ipad"

# create backup dir for alice
curl -X POST -d '{"Username":"alice", "DestDisk": "/tmp/backup"}' -H "Content-Type: application/json" 127.0.0.1:8000/system/backup

# create hashed password
curl -X POST -d '{"Name":"qiwa", "Password": "246172676f6e32696424763d3139246d3d343039362c743d332c703d312463576c335955427362323176636d466e5a53357362323176643246795a5124514f6c52455862513435315572793752487663334b4f5947565673474a624c724e5a58426f544270596a4900", "Phone": "4084088888", "Email": "qiwa@lomorage.io", "NickName": "qiwa", "HomeDir": "/tmp/qiwa"}' -H "Content-Type: application/json" "127.0.0.1:8000/user"

curl -X POST -d '{"Name":"alice", "Password": "alice123", "Phone": "4084088888", "Email": "alice@lomorage.io", "NickName": "Alice", "HomeDir": "/tmp/alice", "EnableSMB": true}' -H "Content-Type: application/json" "127.0.0.1:8000/user"

# create chromecast user
curl -X POST -d '{"Name":"chromecast", "Password": "alice123", "BotUser": true, "Metadatas":[ 
	{"Name":"ChromecastUUID", "Value": "87a600ca350e3837a2abb42875fe487a"}, 
	{"Name":"ChromecastName", "Value": "Family Room TV"}, 
	{"Name":"ChromecastIP", "Value": "10.0.1.19"}, 
	{"Name":"ChromecastPort", "Value": "8009"}]}' -H "Content-Type: application/json" "127.0.0.1:8000/user?token=1234567"
