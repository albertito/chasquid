// Package safeio implements convenient I/O routines that provide additional
// levels of safety in the presence of unexpected failures.
package safeio

import (
	"io/ioutil"
	"os"
	"path"
	"syscall"
)

// WriteFile writes data to a file named by filename, atomically.
// It's a wrapper to ioutil.WriteFile, but provides atomicity (and increased
// safety) by writing to a temporary file and renaming it at the end.
//
// Note this relies on same-directory Rename being atomic, which holds in most
// reasonably modern filesystems.
func WriteFile(filename string, data []byte, perm os.FileMode) error {
	// Note we create the temporary file in the same directory, otherwise we
	// would have no expectation of Rename being atomic.
	// We make the file names start with "." so there's no confusion with the
	// originals.
	tmpf, err := ioutil.TempFile(path.Dir(filename), "."+path.Base(filename))
	if err != nil {
		return err
	}

	if err = tmpf.Chmod(perm); err != nil {
		tmpf.Close()
		os.Remove(tmpf.Name())
		return err
	}

	if uid, gid := getOwner(filename); uid >= 0 {
		if err = tmpf.Chown(uid, gid); err != nil {
			tmpf.Close()
			os.Remove(tmpf.Name())
			return err
		}
	}

	if _, err = tmpf.Write(data); err != nil {
		tmpf.Close()
		os.Remove(tmpf.Name())
		return err
	}

	if err = tmpf.Close(); err != nil {
		os.Remove(tmpf.Name())
		return err
	}

	return os.Rename(tmpf.Name(), filename)
}

func getOwner(fname string) (uid, gid int) {
	uid = -1
	gid = -1
	stat, err := os.Stat(fname)
	if err == nil {
		if sysstat, ok := stat.Sys().(*syscall.Stat_t); ok {
			uid = int(sysstat.Uid)
			gid = int(sysstat.Gid)
		}
	}

	return
}
