//go:build bundled_ai

package main

import _ "embed"

//go:embed quality_bundle/windows.zip
var bundledQualityEngine []byte
