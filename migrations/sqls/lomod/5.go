package lomod

var sql5 = `
CREATE TABLE IF NOT EXISTS assets_hide(
  share_id INTEGER NOT NULL,
  receiver_id INTEGER NOT NULL,
	group_id INTEGER NOT NULL,
  create_time INTEGER NOT NULL,

  UNIQUE (receiver_id, share_id)
);
`
