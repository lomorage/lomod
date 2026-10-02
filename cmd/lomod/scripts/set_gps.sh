#!/bin/bash

db=""
if [ -z "$1" ]
then
  db="../var/assets.db"
else
  db=$1
fi

declare -a longitude=(-122.456283333333 -122.456269444444 -122.45611 -122.449875 -122.507530555556 -122.516625 108.904830555556 120.611816666667 120.611977777778 120.6116 -121.916427612305 -121.77449798584 -121.509811111111 -122.018508333333 -122.018508333333 -121.509811401367)
declare -a latitude=(37.8725055555556 37.8725138888889 37.8725083333333 37.8850472222222 37.586075 37.5721944444444 34.2404444444444 31.2203416666667 31.2205472222222 31.2201 37.3658142089844 37.3282012939453 41.7143833333333 41.81588888889 41.81588888889 41.7143821716309)

sqlite3 $db " UPDATE asset SET 
longitude=CASE
WHEN id=1 THEN ${longitude[0]}
WHEN id=2 THEN ${longitude[1]}
WHEN id=3 THEN ${longitude[2]}
WHEN id=4 THEN ${longitude[3]}
WHEN id=5 THEN ${longitude[4]}
WHEN id=6 THEN ${longitude[5]}
WHEN id=7 THEN ${longitude[6]}
WHEN id=8 THEN ${longitude[7]}
WHEN id=9 THEN ${longitude[8]}
WHEN id=10 THEN ${longitude[9]}
WHEN id=11 THEN ${longitude[10]}
WHEN id=12 THEN ${longitude[11]}
WHEN id=13 THEN ${longitude[12]}
WHEN id=14 THEN ${longitude[13]}
WHEN id=15 THEN ${longitude[14]}
      ELSE ${longitude[15]}  END,
latitude=CASE
WHEN id=1 THEN ${latitude[0]}
WHEN id=2 THEN ${latitude[1]}
WHEN id=3 THEN ${latitude[2]}
WHEN id=4 THEN ${latitude[3]}
WHEN id=5 THEN ${latitude[4]}
WHEN id=6 THEN ${latitude[5]}
WHEN id=7 THEN ${latitude[6]}
WHEN id=8 THEN ${latitude[7]}
WHEN id=9 THEN ${latitude[8]}
WHEN id=10 THEN ${latitude[9]}
WHEN id=11 THEN ${latitude[10]}
WHEN id=12 THEN ${latitude[11]}
WHEN id=13 THEN ${latitude[12]}
WHEN id=14 THEN ${latitude[13]}
WHEN id=15 THEN ${latitude[14]}
      ELSE ${latitude[15]}  END
 WHERE id IN (1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16);"

sqlite3 $db "
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.place.en_US','Crater Lake National Park',1,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.zipcode.en_US','97624',1,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.city.en_US','Chiloquin',1,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.state.en_US','OR',1,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.mail.en_US','Rim Dr
Chiloquin OR 97624
United States',1,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.country.en_US','United States',1,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.street.en_US','Rim Dr',1,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.place.en_US','',2,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.country.en_US','United States',2,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.state.en_US','CA',2,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.zipcode.en_US','96058',2,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.street.en_US','US-97',2,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.city.en_US','Macdoel',2,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.mail.en_US','US-97
Macdoel CA 96058
United States',2,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.country.en_US','United States',3,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.street.en_US','Forest Service Road 10',3,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.place.en_US','Lava Beds National Monument',3,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.state.en_US','CA',3,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.zipcode.en_US','96058',3,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.mail.en_US','Forest Service Road 10
Macdoel CA 96058
United States',3,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.city.en_US','Macdoel',3,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.street.en_US','Rim Dr',4,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.zipcode.en_US','97626',4,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.place.en_US','Crater Lake National Park',4,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.state.en_US','OR',4,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.city.en_US','Fort Klamath',4,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.mail.en_US','Rim Dr
Fort Klamath OR 97626
United States',4,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.country.en_US','United States',4,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.place.en_US','Crater Lake National Park',5,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.country.en_US','United States',5,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.city.en_US','Chiloquin',5,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.mail.en_US','Rim Dr
Chiloquin OR 97624
United States',5,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.state.en_US','OR',5,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.zipcode.en_US','97624',5,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.street.en_US','Rim Dr',5,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.state.en_US','Shaanxi',6,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.place.en_US','',6,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.mail.en_US','China
Shaanxi
Xian
Gaoxin Road',6,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.country.en_US','China',6,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.zipcode.en_US','',6,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.city.en_US','Xian',6,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.street.en_US','Gaoxin Road',6,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.country.en_US','United States',7,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.street.en_US','Forest Service Road 10',7,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.place.en_US','Lava Beds National Monument',7,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.mail.en_US','Forest Service Road 10
Macdoel CA 96058
United States',7,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.state.en_US','CA',7,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.city.en_US','Macdoel',7,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.zipcode.en_US','96058',7,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.zipcode.en_US','95110',8,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.mail.en_US','1730 Technology Dr
San Jose CA 95110
United States',8,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.country.en_US','United States',8,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.city.en_US','San Jose',8,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.street.en_US','Technology Dr',8,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.place.en_US','',8,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.state.en_US','CA',8,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.mail.en_US','3606 Cobbert Dr
San Jose CA 95148
United States',9,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.zipcode.en_US','95148',9,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.city.en_US','San Jose',9,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.country.en_US','United States',9,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.place.en_US','',9,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.street.en_US','Cobbert Dr',9,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.state.en_US','CA',9,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.street.en_US','梧桐街',10,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.country.en_US','China',10,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.place.en_US','',10,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.zipcode.en_US','',10,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.state.en_US','Jiangsu',10,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.mail.en_US','China
Jiangsu
Suzhou
梧桐街',10,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.city.en_US','Suzhou',10,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.zipcode.en_US','94920',11,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.place.en_US','Old St. Hilary Open Space',11,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.street.en_US','',11,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.mail.en_US','Belvedere Tiburon CA 94920
United States',11,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.city.en_US','Belvedere Tiburon',11,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.country.en_US','United States',11,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.state.en_US','CA',11,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.state.en_US','CA',12,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.zipcode.en_US','96058',12,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.mail.en_US','US-97
Macdoel CA 96058
United States',12,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.city.en_US','Macdoel',12,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.country.en_US','United States',12,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.street.en_US','US-97',12,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.place.en_US','',12,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.city.en_US','Pacifica',13,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.place.en_US','Golden Gate National Recreation Area',13,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.state.en_US','CA',13,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.mail.en_US','South Ridge Trail
Pacifica CA 94044
United States',13,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.country.en_US','United States',13,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.street.en_US','South Ridge Trail',13,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.zipcode.en_US','94044',13,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.zipcode.en_US','94920',14,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.country.en_US','United States',14,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.city.en_US','Tiburon',14,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.mail.en_US','23 Main St
Tiburon CA 94920
United States',14,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.street.en_US','Main St',14,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.state.en_US','CA',14,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.place.en_US','',14,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.street.en_US','Main St',15,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.country.en_US','United States',15,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.mail.en_US','23 Main St
Tiburon CA 94920
United States',15,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.place.en_US','',15,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.zipcode.en_US','94920',15,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.state.en_US','CA',15,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.city.en_US','Tiburon',15,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.city.en_US','Tiburon',16,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.mail.en_US','23 Main St
Tiburon CA 94920
United States',16,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.place.en_US','',16,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.street.en_US','Main St',16,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.country.en_US','United States',16,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.state.en_US','CA',16,date('now'),date('now'),0,'',1);
insert into metadata_geo (name,value,asset_id,create_time, last_modified_time,source_device,model,version) values('ios.geo.zipcode.en_US','94920',16,date('now'),date('now'),0,'',1);
"
