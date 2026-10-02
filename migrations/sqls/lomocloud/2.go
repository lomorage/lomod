package lomocloud

var sql2 = `
alter table ip_map add column os TEXT default '';
alter table ip_map add column arch TEXT default '';
alter table ip_map add column lomod_ver TEXT default '';
`
