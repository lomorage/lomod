#!/bin/bash

db=""
if [ -z "$1" ]
then
  db="../var/assets.db"
else
  db=$1
fi

sqlite3 $db "
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,1,'ios.vision.classify.confi','0.26392794','',1,'2021-10-25 21:00:06.122017451+00:00','2021-10-25 21:00:06.122018805+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,1,'ios.vision.classify.label','diaper, nappy, napkin','',1,'2021-10-25 21:00:06.12445338+00:00','2021-10-25 21:00:06.124454942+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,2,'ios.vision.classify.confi','0.42612788','',1,'2021-09-07 05:38:40.879891631+00:00','2021-09-07 05:38:40.87989486+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,2,'ios.vision.classify.label','envelope','',1,'2021-09-07 05:38:40.883968913+00:00','2021-09-07 05:38:40.883973809+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,3,'ios.vision.classify.confi','0.3049954','',1,'2021-09-09 02:19:18.8527149+00:00','2021-09-09 02:19:18.852717921+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,3,'ios.vision.classify.label','breakwater, groin, groyne, mole, bulwark, seawall, jetty','',1,'2021-09-09 02:19:18.856645623+00:00','2021-09-09 02:19:18.856648071+00
:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,4,'ios.vision.classify.confi','0.40289977','',1,'2021-10-24 05:12:13.49343502+00:00','2021-10-24 05:12:13.493438457+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,4,'ios.vision.classify.label','suspension bridge','',1,'2021-10-24 05:12:13.491086991+00:00','2021-10-24 05:12:13.49108871+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,5,'ios.vision.classify.confi','0.47214332','',1,'2021-09-09 05:09:37.110333072+00:00','2021-09-09 05:09:37.110335311+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,5,'ios.vision.classify.label','crib, cot','',1,'2021-09-09 05:09:37.107711362+00:00','2021-09-09 05:09:37.107713237+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,6,'ios.vision.classify.confi','0.28204197','',1,'2021-09-07 05:37:19.569164634+00:00','2021-09-07 05:37:19.569167759+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,6,'ios.vision.classify.label','crib, cot','',1,'2021-09-07 05:37:19.565789538+00:00','2021-09-07 05:37:19.565791934+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,7,'ios.vision.classify.confi','0.3903569','',1,'2021-09-09 14:22:22.59949076+00:00','2021-09-09 14:22:22.599494302+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,7,'ios.vision.classify.label','breakwater, groin, groyne, mole, bulwark, seawall, jetty','',1,'2021-09-09 14:22:22.596413268+00:00','2021-09-09 14:22:22.596415612+00
:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,8,'ios.vision.classify.confi','0.73931575','',1,'2021-09-07 05:46:41.915198583+00:00','2021-09-07 05:46:41.915200927+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,8,'ios.vision.classify.label','seashore, coast, seacoast, sea-coast','',1,'2021-09-07 05:46:41.912132229+00:00','2021-09-07 05:46:41.912134365+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,9,'ios.vision.classify.confi','0.5991356','',1,'2021-09-07 05:29:41.021423891+00:00','2021-09-07 05:29:41.021428839+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,9,'ios.vision.classify.label','seashore, coast, seacoast, sea-coast','',1,'2021-09-07 05:29:41.025682631+00:00','2021-09-07 05:29:41.025685964+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,10,'ios.vision.classify.confi','0.20602088','',1,'2021-09-07 05:33:46.839001791+00:00','2021-09-07 05:33:46.839006947+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,10,'ios.vision.classify.label','bucket, pail','',1,'2021-09-07 05:33:46.83406024+00:00','2021-09-07 05:33:46.834063313+00:00');

INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,11,'ios.vision.classify.confi','0.3460385','',1,'2021-09-07 05:28:52.783001536+00:00','2021-09-07 05:28:52.783003672+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,11,'ios.vision.classify.label','packet','',1,'2021-09-07 05:28:52.778667901+00:00','2021-09-07 05:28:52.778672172+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,12,'ios.vision.classify.confi','0.57275677','',1,'2021-09-07 05:36:51.565688616+00:00','2021-09-07 05:36:51.565691741+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,12,'ios.vision.classify.label','refrigerator, icebox','',1,'2021-09-07 05:36:51.561041335+00:00','2021-09-07 05:36:51.561044877+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,13,'ios.vision.classify.confi','0.22276734','',1,'2021-09-07 05:30:41.353476386+00:00','2021-09-07 05:30:41.35347873+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,13,'ios.vision.classify.label','theater curtain, theatre curtain','',1,'2021-09-07 05:30:41.34987577+00:00','2021-09-07 05:30:41.349877801+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,14,'ios.vision.classify.confi','0.7667954','',1,'2021-09-07 05:38:44.079360951+00:00','2021-09-07 05:38:44.07936418+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,14,'ios.vision.classify.label','cash machine, cash dispenser, automated teller machine, automatic teller machine, automated teller, automatic teller, ATM','',1,'2021-
09-07 05:38:44.082260735+00:00','2021-09-07 05:38:44.082263027+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,15,'ios.vision.classify.confi','0.7337795','',1,'2021-09-07 05:34:59.206308104+00:00','2021-09-07 05:34:59.206310083+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,15,'ios.vision.classify.label','cash machine, cash dispenser, automated teller machine, automatic teller machine, automated teller, automatic teller, ATM','',1,'2021-
09-07 05:34:59.203049049+00:00','2021-09-07 05:34:59.203051341+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,16,'ios.vision.classify.confi','0.86137986','',1,'2021-10-10 22:35:47.774154523+00:00','2021-10-10 22:35:47.774156762+00:00');
INSERT INTO metadata_scene (source_device,asset_id,name,value,model,version,create_time, last_modified_time) VALUES(0,16,'ios.vision.classify.label','cash machine, cash dispenser, automated teller machine, automatic teller machine, automated teller, automatic teller, ATM','',1,'2021-
10-10 22:35:47.770023496+00:00','2021-10-10 22:35:47.770025631+00:00');
"