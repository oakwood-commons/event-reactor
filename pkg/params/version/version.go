package version

import "fmt"

var (
	BuildVersion = "dev"
	Commit       = "none"
	BuildTime    = "unknown"
)

func String() string {
	return fmt.Sprintf("event-reactor %s (commit: %s, built: %s)", BuildVersion, Commit, BuildTime)
}
