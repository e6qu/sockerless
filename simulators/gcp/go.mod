module github.com/sockerless/simulator-gcp

go 1.25.0

require (
	cloud.google.com/go/logging v1.13.0
	github.com/sockerless/simulator v0.0.0
	google.golang.org/genproto/googleapis/api v0.0.0-20260226221140-a57be14db171
	google.golang.org/grpc v1.79.1
	google.golang.org/protobuf v1.36.11
)

require (
	cloud.google.com/go/longrunning v0.6.2 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rs/zerolog v1.33.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	google.golang.org/genproto v0.0.0-20241118233622-e639e219e697 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260217215200-42d3e9bedb6d // indirect
	modernc.org/libc v1.70.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
	modernc.org/sqlite v1.48.1 // indirect
)

replace github.com/sockerless/simulator => ./shared
