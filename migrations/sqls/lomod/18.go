package lomod

var sql18 = `
CREATE TABLE metadata_recface(
  source_device INTEGER NOT NULL,
  asset_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  value TEXT NOT NULL,
  model TEXT NOT NULL,
  version INTEGER NOT NULL,
  create_time INTEGER NOT NULL,
  last_modified_time INTEGER NOT NULL,
  
  UNIQUE (source_device, asset_id, name, model, version)
);

alter table album add column cover_image TEXT default '';
`
