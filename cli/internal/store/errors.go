package store

import "errors"

// ErrNotFound is returned by lookups that find no matching row.
var ErrNotFound = errors.New("store: not found")
