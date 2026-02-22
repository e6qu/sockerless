module github.com/sockerless/frontend

go 1.23.0

toolchain go1.24.2

require (
	github.com/rs/zerolog v1.33.0
	github.com/sockerless/api v0.0.0
)

require (
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	golang.org/x/sys v0.34.0 // indirect
)

replace github.com/sockerless/api => ../../api
