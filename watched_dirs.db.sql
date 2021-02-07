BEGIN TRANSACTION;
CREATE TABLE IF NOT EXISTS "process_states" (
	"id"	INTEGER NOT NULL UNIQUE,
	"state"	TEXT NOT NULL UNIQUE,
	PRIMARY KEY("id" AUTOINCREMENT)
);
CREATE TABLE IF NOT EXISTS "watched_dirs" (
	"id"	INTEGER NOT NULL UNIQUE,
	"path_to_dir"	TEXT NOT NULL UNIQUE,
	"state_id"	INTEGER NOT NULL,
	PRIMARY KEY("id" AUTOINCREMENT)
);
INSERT INTO "process_states" ("id","state") VALUES (1,'indexing');
INSERT INTO "process_states" ("id","state") VALUES (2,'wiping');
INSERT INTO "process_states" ("id","state") VALUES (3,'updating');
CREATE VIEW "watched_dirs_states" AS SELECT
	watched_dirs.id,
	watched_dirs.path_to_dir,
	process_states.state as state
FROM
	watched_dirs
INNER JOIN process_states ON watched_dirs.state_id = process_states.id;
COMMIT;
