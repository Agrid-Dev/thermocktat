package buildinfo

import "fmt"

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func String() string {
	return fmt.Sprintf("thermocktat %s (commit=%s, built=%s)", Version, Commit, Date)
}
