//go:build !unix

package trace

import (
	"errors"
	"os"
	"time"
)

// Windows locking is M1 (doc 09 section 5); until then the lock is an
// exclusive-create spin with a bounded wait.
func acquireLock(path string) (*os.File, error) {
	lockPath := path + ".pid"
	deadline := time.Now().Add(5 * time.Second)
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, os.ErrExist) || time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func releaseLock(f *os.File) error {
	name := f.Name()
	f.Close()
	return os.Remove(name)
}
