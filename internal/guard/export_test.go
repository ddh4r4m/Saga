package guard

import "github.com/ddh4r4m/saga/internal/canon"

func canonPkgSHA(b []byte) string { return canon.SHA256(b) }
