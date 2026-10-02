package lomocloud

var sql1 = `
alter table ip_map add column uuid TEXT default '';
alter table ip_map add column port INTEGER default 0;
alter table ip_map add column name TEXT default '';
`
