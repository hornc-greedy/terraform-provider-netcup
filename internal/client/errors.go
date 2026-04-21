// SPDX-License-Identifier: MPL-2.0

package client

import "fmt"

// APIError is returned whenever the CCP envelope reports status != "success".
type APIError struct {
	Action       string
	Status       string
	StatusCode   int
	ShortMessage string
	LongMessage  string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("netcup %s failed (status %d): %s — %s",
		e.Action, e.StatusCode, e.ShortMessage, e.LongMessage)
}
