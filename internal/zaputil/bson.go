// Copyright 2020 Canonical Ltd.

package zaputil

import (
	"time"

	"github.com/juju/mgo/v2/bson"
	"go.uber.org/zap/zapcore"
)

func BSON(key string, v interface{}) zapcore.Field {
	return zapcore.Field{
		Key:       key,
		Type:      zapcore.ObjectMarshalerType,
		Interface: bsonField{v},
	}
}

type bsonField struct {
	v interface{}
}

// MarshalLogObject implements the zapcore.ObjectMarshaler interface.
func (f bsonField) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	b, err := bson.Marshal(f.v)
	if err != nil {
		return err
	}
	var doc bson.D
	if err := bson.Unmarshal(b, &doc); err != nil {
		return err
	}
	return bsonD{doc}.MarshalLogObject(enc)
}

type bsonD struct {
	d bson.D
}

// MarshalLogObject implements the zapcore.ObjectMarshaler interface.
func (d bsonD) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	for _, d := range d.d {
		switch v := d.Value.(type) {
		case bson.D:
			if err := enc.AddObject(d.Name, bsonD{v}); err != nil {
				return err
			}
		case []interface{}:
			if err := enc.AddArray(d.Name, bsonA{v}); err != nil {
				return err
			}
		case bool:
			enc.AddBool(d.Name, v)
		case float64:
			enc.AddFloat64(d.Name, v)
		case int:
			enc.AddInt(d.Name, v)
		case int64:
			enc.AddInt64(d.Name, v)
		case []byte:
			enc.AddBinary(d.Name, v)
		case string:
			enc.AddString(d.Name, v)
		case time.Time:
			enc.AddTime(d.Name, v)
		default:
			if err := enc.AddReflected(d.Name, v); err != nil {
				return err
			}
		}
	}
	return nil
}

type bsonA struct {
	a []interface{}
}

// MarshalLogArray implements the zapcore.ArrayMarshaler interface.
func (a bsonA) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	for _, av := range a.a {
		switch v := av.(type) {
		case bson.D:
			if err := enc.AppendObject(bsonD{v}); err != nil {
				return err
			}
		case []interface{}:
			if err := enc.AppendArray(bsonA{v}); err != nil {
				return err
			}
		case bool:
			enc.AppendBool(v)
		case float64:
			enc.AppendFloat64(v)
		case int:
			enc.AppendInt(v)
		case int64:
			enc.AppendInt64(v)
		case string:
			enc.AppendString(v)
		case time.Time:
			enc.AppendTime(v)
		default:
			if err := enc.AppendReflected(v); err != nil {
				return err
			}
		}
	}
	return nil
}
