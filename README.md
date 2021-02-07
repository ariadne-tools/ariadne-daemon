# Ariadne
Ariadne is a file indexing and finding tool
Ariadne-daemon creates and maintains indexes of the chosen directories. You can communicate with it via rpc.

## To compile the tool on Linux:
* install golang
* install sqlite3
* go get github.com/mattn/go-sqlite3
* go get github.com/rjeczalik/notify
* git clone https://github.com/ariadne-tools/ariadne-daemon (or download the zip and extract)
* sqlite3 Ariadne/ariadne-daemon/files.db < Ariadne/ariadne-daemon/files.db.sql
* sqlite3 Ariadne/ariadne-daemon/watched_dirs.db < Ariadne/ariadne-daemon/watched_dirs.db.sql
* cd Ariadne/ariadne-daemon/ && go build
