package jsonrpc

import (
	"github.com/ariadne-tools/ariadne-daemon/internal/dbconnect"
	"github.com/ariadne-tools/ariadne-daemon/internal/terminator"
)

type RemoteCall struct {
	Watcheddb *dbconnect.DbConnector
	Filesdb   *dbconnect.DbConnector
}

type FileProperties struct {
	Path_to_file string
	Fname        string
	Size         int
	Mtime_ns     int
	IsDir        bool
}

type WatchedDirsState struct {
	Id    int
	Path  string
	State string
}

func (r RemoteCall) Search(searchString string, files *[]FileProperties) error {
	rows := r.Filesdb.Query("SELECT path_to_file,fname,size,mtime_ns,is_dir FROM files WHERE fname LIKE '%'||?||'%'", searchString)
	for _, row := range rows {
		path_to_file := row[0].(string)
		fname := row[1].(string)
		size := int(row[2].(int64))
		mtime_ns := int(row[3].(int64))

		is_dir_int := row[4].(int64)
		is_dir := is_dir_int != 0

		*files = append(*files, FileProperties{path_to_file, fname, size, mtime_ns, is_dir})
	}
	return nil
}

func (r RemoteCall) StopDaemon(x struct{}, y *struct{}) error {
	terminator.Terminator()
	return nil
}

func (r RemoteCall) Add(dirpaths []string, added *[]string) error {

	q := r.Watcheddb.Query("SELECT path_to_dir FROM watched_dirs")
	dirsAdded := make([]string, 0)

	for _, v := range q {
		dirsAdded = append(dirsAdded, v[0].(string))
	}

	for _, dirpath := range dirpaths {

		alreadyAdded := false
		for _, added := range dirsAdded {
			if added == dirpath {
				alreadyAdded = true
			}
		}
		if !alreadyAdded {
			r.Watcheddb.Exec("INSERT into watched_dirs (path_to_dir, state_id) VALUES (?,?)", dirpath, 1)
			*added = append(*added, dirpath)
		}
	}
	return nil
}

func (r RemoteCall) Remove(dirIds []int, removed *[]int) error {
	watched := dbconnect.WatchedIds(r.Watcheddb)
	for _, dirId := range dirIds {
		if _, in := watched[dirId]; in == true {
			r.Watcheddb.Exec("UPDATE watched_dirs SET state_id=? WHERE id=?", 2, dirId) //TODO: 2-t cserelni
			*removed = append(*removed, dirId)
		}
	}
	return nil
}

func (r RemoteCall) WatchedDirs(_ struct{}, watched *[]WatchedDirsState) error {
	dirs := r.Watcheddb.Query("SELECT * FROM watched_dirs_states")
	for _, dir := range dirs {
		id := int(dir[0].(int64))
		path := dir[1].(string)
		state := dir[2].(string)
		*watched = append(*watched, WatchedDirsState{id, path, state})
	}
	return nil
}
