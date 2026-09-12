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
	github.com/descope/virtualwebauthn v1.0.5
	github.com/go-chi/chi/v5 v5.3.2
	github.com/go-webauthn/webauthn v0.18.1
	github.com/google/nftables v0.3.0
	github.com/gorilla/sessions v1.4.0
	github.com/mdlayher/netlink v1.11.2
	github.com/nicksnyder/go-i18n/v2 v2.6.1
	golang.org/x/crypto v0.57.0
	golang.org/x/sys v0.48.0
	golang.org/x/text v0.42.0
	golang.org/x/time v0.16.0
)

require (
	github.com/fxamacker/cbor/v2 v2.9.3 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/go-webauthn/x v0.3.1 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/securecookie v1.1.2 // indirect
	github.com/mdlayher/socket v0.6.0 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/tinylib/msgp v1.6.4 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
)
