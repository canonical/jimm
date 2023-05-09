// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"

	jujuparams "github.com/juju/juju/rpc/params"
)

// JSON is a custom type implementing the Value() and Scan() methods for
// reading/writing with Gorm. Implementations have been reused from
// https://github.com/go-gorm/datatypes
type JSON json.RawMessage

// Value return json value, implement driver.Valuer interface
func (j JSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	return string(j), nil
}

// Scan scan value into Jsonb, implements sql.Scanner interface
func (j *JSON) Scan(value interface{}) error {
	if value == nil {
		*j = JSON("null")
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		if len(v) > 0 {
			bytes = make([]byte, len(v))
			copy(bytes, v)
		}
	case string:
		bytes = []byte(v)
	default:
		return errors.New(fmt.Sprint("Failed to unmarshal JSONB value:", value))
	}

	result := json.RawMessage(bytes)
	*j = JSON(result)
	return nil
}

// GormDataType gorm common data type
func (JSON) GormDataType() string {
	return "bytes"
}

// Strings is a data type that stores a slice of strings into a single
// column. The strings are encoded as a JSON array and stored in a BLOB
// data type.
type Strings []string

// GormDataType implements schema.GormDataTypeInterface.
func (s Strings) GormDataType() string {
	return "bytes"
}

// Value implements driver.Valuer.
func (s Strings) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

// FromPointer sets the Strings value to be that of the given pointer to a
// string slice.
func (s *Strings) FromPointer(sp *[]string) {
	if sp == nil {
		*s = nil
		return
	}
	*s = Strings(*sp)
}

// Scan implements sql.Scanner.
func (s *Strings) Scan(src interface{}) error {
	if src == nil {
		*s = nil
		return nil
	}
	var buf []byte
	switch v := src.(type) {
	case []byte:
		buf = v
	case string:
		buf = []byte(v)
	default:
		return fmt.Errorf("cannot unmarshal %T as Strings", src)
	}
	return json.Unmarshal(buf, s)
}

// A StringMap is a data type that flattens a map of string to string into
// a single column. The map is encoded as a JSON object and stored in a
// BLOB data type.
type StringMap map[string]string

// GormDataType implements schema.GormDataTypeInterface.
func (m StringMap) GormDataType() string {
	return "bytes"
}

// Value implements driver.Valuer.
func (m StringMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

// Scan implements sql.Scanner.
func (m *StringMap) Scan(src interface{}) error {
	if src == nil {
		*m = nil
		return nil
	}
	var buf []byte
	switch v := src.(type) {
	case []byte:
		buf = v
	case string:
		buf = []byte(v)
	default:
		return fmt.Errorf("cannot unmarshal %T as StringMap", src)
	}
	return json.Unmarshal(buf, m)
}

// A Map stores a generic map in a database column. The map is encoded as
// JSON and stored in a BLOB element.
type Map map[string]interface{}

// GormDataType implements schema.GormDataTypeInterface.
func (m Map) GormDataType() string {
	return "bytes"
}

// Value implements driver.Valuer.
func (m Map) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

// Scan implements sql.Scanner.
func (m *Map) Scan(src interface{}) error {
	if src == nil {
		*m = nil
		return nil
	}
	var buf []byte
	switch v := src.(type) {
	case []byte:
		buf = v
	case string:
		buf = []byte(v)
	default:
		return fmt.Errorf("cannot unmarshal %T as Map", src)
	}
	return json.Unmarshal(buf, m)
}

// HostPorts is data type that stores a set of jujuparams.HostPort in a
// single column. The hostports are encoded as JSON and stored in a BLOB
// value.
type HostPorts [][]jujuparams.HostPort

// GormDataType implements schema.GormDataTypeInterface.
func (HostPorts) GormDataType() string {
	return "bytes"
}

// Value implements driver.Valuer.
func (hp HostPorts) Value() (driver.Value, error) {
	if hp == nil {
		return nil, nil
	}
	// It would normally be bad practice to directly encode exernal
	// data-types one doesn't control in the database, but in this case
	// it is probably fine because it is part of the published API and
	// therefore is unlikely to change in an incompatible way.
	return json.Marshal(hp)
}

// Scan implements sql.Scanner.
func (hp *HostPorts) Scan(src interface{}) error {
	if src == nil {
		*hp = nil
		return nil
	}
	var buf []byte
	switch v := src.(type) {
	case []byte:
		buf = v
	case string:
		buf = []byte(v)
	default:
		return fmt.Errorf("cannot unmarshal %T as HostPorts", src)
	}
	return json.Unmarshal(buf, hp)
}

// SetNullString sets ns to a valid string with the value of *s if s is not
// nil, otherwise ns is set to be invalid.
func SetNullString(ns *sql.NullString, s *string) {
	ns.Valid = s != nil
	if ns.Valid {
		ns.String = *s
	} else {
		ns.String = ""
	}
}
