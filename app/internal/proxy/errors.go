package proxy

import "errors"

var errNotFound = errors.New("flow not found")

// ErrNotFound is returned when a referenced flow does not exist.
var ErrNotFound = errNotFound
