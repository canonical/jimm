package version

// init is used to update the version info with the correct details for
// the current build. It is expected that an appropriate build script
// or Makefile will create a new init.go file based on this template
// using a command like the following:
//
// gofmt -r "unknownVersion -> Version{GitCommit: \"${GIT_COMMIT}\", Version: \"${VERSION}\",}" init.go.tmpl > init.go
func init() {
	VersionInfo = Version{GitCommit: "90b3bc73d51d53bfb0d7c1e46924e3917ca1aac2", Version: ""}
}
