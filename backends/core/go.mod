module github.com/sockerless/backend-core

go 1.24.2

require (
	github.com/gorilla/websocket v1.5.3
	github.com/rs/zerolog v1.33.0
	github.com/sockerless/agent v0.0.0
	github.com/sockerless/api v0.0.0
)

require (
	github.com/creack/pty v1.1.24 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	golang.org/x/sys v0.38.0 // indirect
)

replace (
	github.com/sockerless/agent => ../../agent
	github.com/sockerless/api => ../../api
)
