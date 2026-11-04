package wa

import (
	"net/url"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go SQLite driver ("sqlite"): no cgo, cross-compiles everywhere
)

// sqlDriver is the database/sql driver name registered by modernc.org/sqlite.
// whatsmeow's dialect parser accepts any name starting with "sqlite".
const sqlDriver = "sqlite"

// sessionDSN opens path with foreign keys on (whatsmeow refuses to upgrade
// without them), a 5 s busy timeout and immediate write transactions. The
// journal mode stays SQLite's default, so a whatsapp.db created by an older
// (mattn/cgo) build opens unchanged and no -wal/-shm sidecars appear.
func sessionDSN(path string) string {
	u := url.URL{Path: filepath.ToSlash(path)} // escapes % ? # in the path; C:/... works on Windows
	return "file:" + u.EscapedPath() + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_txlock=immediate"
}
