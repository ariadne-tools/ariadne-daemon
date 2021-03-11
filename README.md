# Ariadne
Ariadne is a file indexing and finding tool
Ariadne-daemon creates and maintains indexes of the chosen directories. You can communicate with it via rpc.

## To compile the tool on Linux:
* go run build.go
* sqlite3 Ariadne/ariadne-daemon/files.db < Ariadne/ariadne-daemon/files.db.sql
* sqlite3 Ariadne/ariadne-daemon/watched_dirs.db < Ariadne/ariadne-daemon/watched_dirs.db.sql