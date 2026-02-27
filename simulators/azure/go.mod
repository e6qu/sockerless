module github.com/sockerless/simulator-azure

go 1.25.0

require github.com/sockerless/simulator v0.0.0

require (
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/rs/zerolog v1.33.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
)

replace github.com/sockerless/simulator => ./shared
