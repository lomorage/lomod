package lomod

// sql20 keeps the name the file had on the device that uploaded it (e.g. IMG_1234.HEIC). The
// file on disk keeps its date+id name; this is only what clients show and what downloads are
// called. Empty for assets uploaded before clients sent it.
var sql20 = `
alter table asset add column orig_name TEXT NOT NULL default '';
`
