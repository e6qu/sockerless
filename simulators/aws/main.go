// Command simulator-aws runs the AWS service simulator.
//
// It simulates the subset of AWS APIs used by the Sockerless ECS and Lambda
// backends: ECS, ECR, CloudWatch Logs, EFS, Cloud Map, Lambda, S3, EC2, IAM, and STS.
//
// Configure with environment variables:
//
//	SIM_LISTEN_ADDR  — listen address (default ":4566")
//	SIM_TLS_CERT     — TLS certificate file (optional)
//	SIM_TLS_KEY      — TLS key file (optional)
//	SIM_LOG_LEVEL    — log level: trace, debug, info, warn, error (default "info")
//
// SDK configuration:
//
//	export AWS_ENDPOINT_URL=http://localhost:4566
//	export AWS_ACCESS_KEY_ID=test
//	export AWS_SECRET_ACCESS_KEY=test
//	export AWS_DEFAULT_REGION=us-east-1
package main

import (
	"log"
	"net/http"
	"os"

	sim "github.com/sockerless/simulator"
)

func main() {
	cfg := sim.ConfigFromEnv("aws")
	if cfg.ListenAddr == ":8443" {
		cfg.ListenAddr = ":4566" // AWS simulator default port
	}

	// Override from SIM_AWS_PORT if set
	if port := os.Getenv("SIM_AWS_PORT"); port != "" {
		cfg.ListenAddr = ":" + port
	}

	srv := sim.NewServer(cfg)

	// Register AWS JSON services (X-Amz-Target header routing)
	awsRouter := sim.NewAWSRouter()
	registerECS(awsRouter, srv)
	registerECR(awsRouter, srv)
	registerCloudWatchLogs(awsRouter, srv)
	registerCloudMap(awsRouter, srv)

	// Register AWS Query Protocol services (Action form parameter routing)
	queryRouter := sim.NewAWSQueryRouter()
	registerEC2(queryRouter)
	registerIAM(queryRouter)
	registerSTS(queryRouter)

	// POST / handler: check X-Amz-Target first (JSON protocol),
	// fall back to Action parameter (Query Protocol)
	srv.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Amz-Target") != "" {
			awsRouter.ServeHTTP(w, r)
			return
		}
		queryRouter.ServeHTTP(w, r)
	})

	// REST-based services register directly on the server mux
	registerEFS(srv)
	registerLambda(srv)
	registerS3(srv)

	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
