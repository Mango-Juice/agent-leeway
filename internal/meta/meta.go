// Package meta holds the product name, version and every release location in one place.
//
// The install scripts and .goreleaser.yaml repeat Owner; a test keeps the scripts in step with it.
package meta

// Version is set at build time from the release tag: -ldflags "-X github.com/Mango-Juice/agent-leeway/internal/meta.Version=0.1.0".
// GoReleaser passes the tag without its "v". A build without the flag reports the development version below.
var Version = "0.1.0-dev"

const (
	// DisplayName is the product name shown to people.
	DisplayName = "Leeway"
	// Owner is the GitHub account that hosts the repository.
	Owner = "Mango-Juice"
	// Repo is the repository name.
	Repo = "agent-leeway"
)

// RepoURL is the project page.
func RepoURL() string { return "https://github.com/" + Owner + "/" + Repo }

// ReleasesURL lists the published releases.
func ReleasesURL() string { return RepoURL() + "/releases" }

// LatestReleaseAPI is read by `self-update --check`.
func LatestReleaseAPI() string {
	return "https://api.github.com/repos/" + Owner + "/" + Repo + "/releases/latest"
}

// InstallSh is the macOS/Linux one-line install (downloads from GitHub Releases, verifies the checksum).
func InstallSh() string {
	return "curl -fsSL https://raw.githubusercontent.com/" + Owner + "/" + Repo + "/main/install/install.sh | sh"
}

// InstallPs1 is the Windows one-line install (user PATH only, no admin rights).
func InstallPs1() string {
	return "irm https://raw.githubusercontent.com/" + Owner + "/" + Repo + "/main/install/install.ps1 | iex"
}
