// Copyright 2020 Canonical Ltd.

package jimmdb

import "github.com/juju/mgo/v2/bson"

// A Query is a helper for creating document queries.
type Query = bson.D

// And creates a query that checks that all the given queries match.
func And(qs ...Query) Query {
	return Query{{Name: "$and", Value: qs}}
}

// ElemMatch creates a query that matches if a single value in the array
// field matches the given value.
func ElemMatch(field string, value interface{}) Query {
	return Query{{Name: field, Value: Query{{Name: "$elemMatch", Value: value}}}}
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

// Ne returns a query that the given field does not match the given value
func Ne(field string, value interface{}) Query {
	return Query{{Name: field, Value: bson.D{{Name: "$ne", Value: value}}}}
}

// NotElemMatch returns a query that the given array field does not have an
// element that matches the given query.
func NotElemMatch(field string, q Query) Query {
	return Query{{Name: field, Value: bson.D{{Name: "$not", Value: bson.D{{Name: "$elemMatch", Value: q}}}}}}
}

// NotExists returns a query that the given field does not exist.
func NotExists(field string) Query {
	return Query{{Name: field, Value: bson.D{{Name: "$exists", Value: false}}}}
}

// Or creates a query that checks if any of the given queries match.
func Or(qs ...Query) Query {
	return Query{{Name: "$or", Value: qs}}
}

// Regex returns a query that the given field matches the given regular
// expression.
func Regex(field, regex string) Query {
	return Query{{Name: field, Value: Query{{Name: "$regex", Value: regex}}}}
}
