// Copyright 2020 Canonical Ltd.

package jimmdb

import "gopkg.in/mgo.v2/bson"

// A Query is a helper for creating document queries.
type Query = bson.D

// And creates a query that checks that all the given queries match.
func And(qs ...Query) Query {
	return Query{{Name: "$and", Value: qs}}
}

// Eq creates a query that checks that the given field has the given value.
func Eq(field string, value interface{}) Query {
	return Query{{Name: field, Value: value}}
}

// Exists returns a query that the given field exists.
func Exists(field string) Query {
	return Query{{Name: field, Value: bson.D{{Name: "$exists", Value: true}}}}
}

// Gte returns a query that the given field is greater-than or equal to the
// given value.
func Gte(field string, value interface{}) Query {
	return Query{{Name: field, Value: bson.D{{Name: "$gte", Value: value}}}}
}

// In returns a query that the given field contains one of the given
// values.
func In(field string, values ...interface{}) Query {
	return Query{{Name: field, Value: bson.D{{Name: "$in", Value: values}}}}
}

// NotExists returns a query that the given field does not exist.
func NotExists(field string) Query {
	return Query{{Name: field, Value: bson.D{{Name: "$exists", Value: false}}}}
}

// Or returns a query that any of the given queries match.
func Or(qs ...Query) Query {
	return Query{{Name: "$or", Value: qs}}
}
