//go:build unix

package store

import (
	"io/fs"
	"syscall"
)

func nlink(fi fs.FileInfo) uint64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Nlink)
	}
	return 1
}
