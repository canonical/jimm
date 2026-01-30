// Package rivertypes defines the arguments for inserting River jobs. This is useful
// for the domain layer to insert with River jobs.
//
// River job args are analogous to a schema for traditional queue systems, defining what
// we will insert into the DB for processing later by workers. By defining them
// in a 3rd package we avoid circular dependencies between the domain layer and the
// River implementation.
package rivertypes
