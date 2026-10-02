package lomod

var sql4 = `
CREATE TABLE IF NOT EXISTS asset_new (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 user_id INTEGER NOT NULL,
 year INTEGER NOT NULL,
 month INTEGER NOT NULL,
 day INTEGER NOT NULL,
 ext_id INTEGER NOT NULL,
 hash TEXT NOT NULL,
 device_id INTEGER NOT NULL,
 create_time INTEGER NOT NULL,
 upload_time INTEGER NOT NULL,

 UNIQUE (user_id, hash)
);

INSERT INTO asset_new SELECT * FROM asset;
DROP TABLE asset;
ALTER TABLE asset_new RENAME TO asset;

alter table asset add column latitude REAL default 0.0;
alter table asset add column longitude REAL default 0.0;
`
