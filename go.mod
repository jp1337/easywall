module github.com/jp1337/easywall

// The minimum this code needs: internal/web/server.go calls
// http.NewCrossOriginProtection, which arrived in Go 1.25. It moves when the code
// starts using a newer API, and not before — raising it costs every consumer.
go 1.26.0

// What we build with. actions/setup-go reads this in preference to the directive
// above, so every workflow, the release and the Debian package compile with
// exactly this toolchain; Renovate keeps the line current on its own, which is
// the whole reason it is here rather than spelled out in ten workflow steps.
toolchain go1.27.1

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/boombuler/barcode v1.1.0
	github.com/go-chi/chi/v5 v5.3.2
	github.com/google/nftables v0.3.0
	github.com/gorilla/sessions v1.4.0
	github.com/nicksnyder/go-i18n/v2 v2.6.1
	golang.org/x/crypto v0.56.0
	golang.org/x/sys v0.47.0
	golang.org/x/text v0.41.0
	golang.org/x/time v0.15.0
)

require (
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/gorilla/securecookie v1.1.2 // indirect
	github.com/mdlayher/netlink v1.7.3-0.20250113171957-fbb4dce95f42 // indirect
	github.com/mdlayher/socket v0.5.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
)
