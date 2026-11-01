// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"errors"
	"net/http"
)

// IsNotFound reports whether err is an *APIError with HTTP 404.
func IsNotFound(err error) bool {
	if apiErr, ok := errors.AsType[*APIError](err); ok {
		return apiErr.Status == http.StatusNotFound
	}
	return false
}

// HasCode reports whether err is an *APIError with the given netcup code.
func HasCode(err error, code string) bool {
	if apiErr, ok := errors.AsType[*APIError](err); ok {
		return apiErr.Code == code
	}
	return false
}
