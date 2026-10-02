#!/bin/bash

for i in `seq 1 $1`;
do
	LOMOD_ENV_UNMOUNT=1 go test -v --tags 'sqlite_trace trace' -check.vv | tee lomod_api_test_loop$i.log
done
