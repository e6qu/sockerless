module github.com/sockerless/simulator-azure-cli-tests

go 1.25.0

require github.com/stretchr/testify v1.11.1

require golang.org/x/sys v0.47.0 // indirect

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/sockerless/simulator-realexec v0.0.0
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/sockerless/simulator-realexec => ../../realexec
