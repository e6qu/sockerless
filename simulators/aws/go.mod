module github.com/sockerless/simulator-aws

go 1.25.0

require (
	github.com/gorilla/websocket v1.5.3
	github.com/sockerless/simulator v0.0.0
)

require (
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/rs/zerolog v1.33.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/sys v0.28.0 // indirect
)

replace github.com/sockerless/simulator => ./shared
