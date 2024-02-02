// Copyright 2024 canonical.

// Package auth provides means to authenticate users into JIMM.
//
// The methods of authentication are:
// - Macaroons (deprecated)
// - OAuth2.0 (Device flow)
// - OAuth2.0 (Browser flow)
// - JWTs (For CLI based sessions)
package auth
