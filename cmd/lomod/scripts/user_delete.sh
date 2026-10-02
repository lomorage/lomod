#!/bin/bash

set -ex

if [ $# -ne 1 ]; 
then
  echo "user_delete.sh <user name>"
  exit 1
fi

cp ../assets.db .

sqlite3 ./assets.db <<END_SQL
delete from user where user_name='$1';
END_SQL

sudo smbpasswd -x $1
sudo userdel -r $1
sudo groupdel $1
