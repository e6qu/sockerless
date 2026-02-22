module github.com/sockerless/backend-memory

go 1.24.2

require (
	github.com/rs/zerolog v1.33.0
	github.com/sockerless/backend-core v0.0.0
	github.com/sockerless/sandbox v0.0.0-00010101000000-000000000000
)

require (
	github.com/creack/pty v1.1.24 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/sockerless/agent v0.0.0 // indirect
	github.com/sockerless/api v0.0.0 // indirect
	github.com/tetratelabs/wazero v1.11.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/term v0.32.0 // indirect
	mvdan.cc/sh/v3 v3.12.0 // indirect
)

replace (
	github.com/sockerless/agent => ../../agent
	github.com/sockerless/api => ../../api
	github.com/sockerless/backend-core => ../core
	github.com/sockerless/sandbox => ../../sandbox
)
