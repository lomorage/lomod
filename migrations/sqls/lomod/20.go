package lomod

// sql20 keeps the name the file had on the device that uploaded it (e.g. IMG_1234.HEIC). The
// file on disk keeps its date+id name; this is only what clients show and what downloads are
// called. Empty for assets uploaded before clients sent it.
//
// orig_name_rev orders names recorded after the upload (POST /assets/names): a per-user counter,
// so other devices can fetch the ones added since they last asked (GET /assets/names?since=).
// Those names don't change the tree hashes, so the tree alone would never tell them. 0 for a
// name that came with the upload, which devices get from the listing. orig_name_counter keeps
// the last revision handed out per user, so deleting assets never lets a revision be reused.
var sql20 = `
alter table asset add column orig_name TEXT NOT NULL default '';
alter table asset add column orig_name_rev INTEGER NOT NULL default 0;
create index if not exists idx_asset_orig_name_rev on asset(user_id, orig_name_rev);
create table if not exists orig_name_counter(
  user_id INTEGER PRIMARY KEY,
  rev INTEGER NOT NULL
);
`
