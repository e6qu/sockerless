module github.com/sockerless/simulator-gcp

go 1.23.0

require (
	cloud.google.com/go/logging v1.13.0
	github.com/sockerless/simulator v0.0.0
	google.golang.org/genproto/googleapis/api v0.0.0-20250519155744-55703ea1f237
	google.golang.org/grpc v1.72.1
	google.golang.org/protobuf v1.36.11
)

require (
	cloud.google.com/go/longrunning v0.6.2 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/rs/zerolog v1.33.0 // indirect
	golang.org/x/net v0.37.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	google.golang.org/genproto v0.0.0-20241118233622-e639e219e697 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250512202823-5a2f75b736a9 // indirect
)

replace github.com/sockerless/simulator => ./shared
