package main

import "encoding/json"

// Release builds pass -X main.appVersion. Plain development builds use the
// version of the bundled extension instead of an obsolete fallback string.
func init() {
 if appVersion != appDefaultVersion { return }
 data, err := pages.ReadFile("browser_extension/manifest.json")
 if err != nil { return }
 var manifest struct { Version string `json:"version"` }
 if json.Unmarshal(data, &manifest) != nil { return }
 if _, err := parseSemver(manifest.Version); err == nil && compareSemver(manifest.Version, appDefaultVersion) > 0 { appVersion = manifest.Version }
}
