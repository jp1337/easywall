// Package config carries the commented default configuration files that ship
// with easywall, embedded into the binaries.
//
// They are embedded rather than copied so there is exactly one of each. These
// two files are what the Debian package installs, what the container image
// copies in, and what `--write-config` writes out — a second copy inside the
// Go tree would be a second thing to keep in step, and this repository has
// already shipped one artefact that drifted from its own description.
//
// This is the only go:embed in the tree, and it embeds configuration, not
// assets: the templates, locales and stylesheets are read from disk at runtime
// so an operator can look at them and change them in place.
package config

import _ "embed"

// Core is config/easywall.toml — the root daemon's configuration.
//
//go:embed easywall.toml
var Core []byte

// Web is config/web.toml — the web process's configuration.
//
//go:embed web.toml
var Web []byte
