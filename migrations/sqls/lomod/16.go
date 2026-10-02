package lomod

var sql16 = `
alter table asset add column exif TEXT default '';

CREATE TABLE camera_make(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  make TEXT NOT NULL,
  model TEXT NOT NULL,
  create_time INTEGER DEFAULT current_timestamp,

  UNIQUE (make, model)
);

CREATE TABLE asset_album_camera_make(
  make_id INTEGER NOT NULL,
  asset_id INTEGER NOT NULL,
  create_time INTEGER DEFAULT current_timestamp,

  UNIQUE (asset_id, make_id)
);
`