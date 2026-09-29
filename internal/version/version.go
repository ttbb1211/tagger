package version

// Version is replaced by the release build through -ldflags:
//   -X github.com/ericwyn/tagger/internal/version.Version=1.5.0
// Keep this fallback in sync with the latest release so a build that forgets
// the flag still reports something close to reality instead of an old number.
var Version = "1.5.0"
