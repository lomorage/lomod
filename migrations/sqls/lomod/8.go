package lomod

var sql8 = `
CREATE TABLE album(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  title TEXT NOT NULL,
  description TEXT NOT NULL,
  author TEXT NOT NULL,
  create_time INTEGER NOT NULL,
  last_modified_time INTEGER NOT NULL,
  
  UNIQUE (user_id, title, author)
);

CREATE TABLE asset_album(
  asset_id INTEGER NOT NULL,
  album_id INTEGER NOT NULL,
  create_time INTEGER NOT NULL,
  
  UNIQUE (asset_id, album_id)
);

`
