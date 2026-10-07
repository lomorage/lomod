package lomod

// sql19 persists consistency check results so clients can ask whether an asset was
// re-verified on disk. Only the latest completed run (end_time not null) is evidence;
// ccheck_bad is keyed by asset id, which AUTOINCREMENT never reuses.
var sql19 = `
CREATE TABLE IF NOT EXISTS ccheck_run(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  start_time INTEGER NOT NULL,
  end_time INTEGER,
  max_asset_id INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS ccheck_bad(
  run_id INTEGER NOT NULL,
  asset_id INTEGER NOT NULL,
  type INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_ccheck_bad_asset ON ccheck_bad(asset_id);
`
