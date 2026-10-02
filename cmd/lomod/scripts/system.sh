#!/bin/sh

curl 127.0.0.1:8000/system | python -m json.tool
curl -X POST 127.0.0.1:8000/system/backup/db/alice/1
curl -X POST 127.0.0.1:8000/system/backup/asset/alice/0

curl 127.0.0.1:8000/system | python -m json.tool
