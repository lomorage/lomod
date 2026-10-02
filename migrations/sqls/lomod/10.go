package lomod

var sql10 = `
alter table asset add column type INTEGER default 0;

alter table asset_album add column orig_filename TEXT default '';
`
