package lomod

var sql3 = `
CREATE TABLE IF NOT EXISTS metadata(
 asset_id INTEGER NOT NULL,
 name TEXT NOT NULL,
 value TEXT NOT NULL,
 create_time INTEGER NOT NULL,
 last_modified_time INTEGER NOT NULL,

 UNIQUE (asset_id, name)
);
`
