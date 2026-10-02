package lomod

var sql7 = `
DROP TABLE metadata;
CREATE TABLE metadata(
  category INTEGER NOT NULL,
  source_device INTEGER NOT NULL,
  asset_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  value TEXT NOT NULL,
  model TEXT NOT NULL,
  version INTEGER NOT NULL,
  create_time INTEGER NOT NULL,
  last_modified_time INTEGER NOT NULL,
  
  UNIQUE (category, source_device, asset_id, name)
);
`
