// Copyright 2018 Canonical Ltd.

package params

import (
	"time"

	"gopkg.in/errgo.v1"
)

// QueryTime decodes date/time parameters in httprequest requests.
type QueryTime struct {
	time.Time
}

const dateFormat = "2006-01-02"

// UnmarshalText implements encoding.TextUnmarshaler.
func (t *QueryTime) UnmarshalText(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	strData := string(data)
	tm, err := time.Parse(time.RFC3339, strData)
	if err == nil {
		t.Time = tm
		return nil
	}
	tm, err = time.Parse(dateFormat, strData)
	if err != nil {
		return errgo.Newf("invalid time format, should be %q or %q: %q", dateFormat, time.RFC3339, string(data))
	}
	t.Time = tm
	return nil
}

// MarshalText implements encoding.TextMarshaler.
func (t QueryTime) MarshalText() ([]byte, error) {
	if t.IsZero() {
		return nil, nil
	}
	return []byte(t.Format(dateFormat)), nil
}
