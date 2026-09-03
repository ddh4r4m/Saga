//go:build !unix

package store

import "io/fs"

func nlink(fi fs.FileInfo) uint64 { return 1 }
