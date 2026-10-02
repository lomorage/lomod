package lomod

var sql11 = `
CREATE TABLE metadata_geo(
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

insert into metadata_geo select 
  source_device,
	asset_id,
	name,
	value,
	model,
	version,
	create_time,
	last_modified_time from metadata where category=0;

CREATE TABLE metadata_scene(
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

insert into metadata_scene select 
  source_device,
	asset_id,
	name,
	value,
	model,
	version,
	create_time,
	last_modified_time from metadata where category=1;

CREATE TABLE metadata_face(
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

insert into metadata_face select 
  source_device,
	asset_id,
	name,
	value,
	model,
	version,
	create_time,
	last_modified_time from metadata where category=2;

CREATE TABLE metadata_text(
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

insert into metadata_text select 
  source_device,
	asset_id,
	name,
	value,
	model,
	version,
	create_time,
	last_modified_time from metadata where category=3;

CREATE TABLE metadata_human(
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

insert into metadata_human select 
  source_device,
	asset_id,
	name,
	value,
	model,
	version,
	create_time,
	last_modified_time from metadata where category=4;

CREATE TABLE metadata_similarity(
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

insert into metadata_similarity select 
  source_device,
	asset_id,
	name,
	value,
	model,
	version,
	create_time,
	last_modified_time from metadata where category=5;

CREATE TABLE metadata_tag(
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

insert into metadata_tag select 
  source_device,
	asset_id,
	name,
	value,
	model,
	version,
	create_time,
	last_modified_time from metadata where category=6;

CREATE TABLE metadata_encrypt(
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

insert into metadata_encrypt select 
  source_device,
	asset_id,
	name,
	value,
	model,
	version,
	create_time,
	last_modified_time from metadata where category=7;
`
