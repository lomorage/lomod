package lomod

var sql13 = `
CREATE TABLE album_geo(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  level int default 1,
  alias TEXT default '',
  country TEXT default '',
  state TEXT default '',
  district TEXT default '',
  city TEXT default '',
  locality TEXT default '',
  neighborhood TEXT default '',
  street TEXT default '',
  substreet TEXT default '',
  poi TEXT default '',
  longitude REAL default 0,
  latitude REAL default 0,
  create_time INTEGER NOT NULL
);

CREATE TABLE album_geo_lang(
  lang TEXT default '',
  name TEXT default '',
  name_en TEXT default ''
);

CREATE TABLE asset_album_geo(
  asset_id INTEGER NOT NULL,
  album_id INTEGER NOT NULL,
  create_time INTEGER NOT NULL,
  
  UNIQUE (asset_id, album_id)
);`