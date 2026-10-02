CREATE TABLE IF NOT EXISTS user (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 user_name TEXT NOT NULL,
 password TEXT NOT NULL,
 phone TEXT NOT NULL,
 email TEXT NOT NULL,
 nick_name TEXT NOT NULL,
 status INTEGER NOT NULL,
 create_time INTEGER NOT NULL,
 last_modified_time INTEGER NOT NULL,
 last_login_time INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS portmap(
 user_id INTEGER NOT NULL,
 subdomain TEXT NOT NULL,
 port_in INTEGER NOT NULL,
 port_out INTEGER NOT NULL,
 ip TEXT NOT NULL,
 create_time INTEGER NOT NULL,
 last_modified_time INTEGER NOT NULL,

 UNIQUE (user_id, port_in, port_out)
);

CREATE TABLE IF NOT EXISTS token(
 token TEXT NOT NULL,
 user_id INTEGER NOT NULL,
 device_id INTEGER NOT NULL,
 create_time INTEGER NOT NULL,
 expire_time INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS device(
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 user_id INTEGER NOT NULL,
 device_name TEXT NOT NULL
);

create table if not exists schema_migrations (
 latest INTEGER PRIMARY KEY NOT NULL ,
 update_time INTEGER NOT NULL
);