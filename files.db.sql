BEGIN TRANSACTION;
CREATE TABLE IF NOT EXISTS "files" (
	"dir_id"	INTEGER NOT NULL,
	"path_to_file"	TEXT NOT NULL,
	"fname"	TEXT NOT NULL,
	"size"	INTEGER NOT NULL,
	"ctime_ns"	INTEGER,
	"mtime_ns"	INTEGER NOT NULL,
	"is_dir" INTEGER NOT NULL,
	PRIMARY KEY("path_to_file","fname")
);
COMMIT;
