package main

import (
	"fmt"
	"net/http"

	sim "github.com/sockerless/simulator"
)

func registerSTS(r *sim.AWSQueryRouter) {
	r.Register("GetCallerIdentity", handleGetCallerIdentity)
}

func handleGetCallerIdentity(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetCallerIdentityResult>
    <Arn>arn:aws:iam::123456789012:user/simulator</Arn>
    <UserId>AKIAIOSFODNN7EXAMPLE</UserId>
    <Account>123456789012</Account>
  </GetCallerIdentityResult>
  <ResponseMetadata><RequestId>%s</RequestId></ResponseMetadata>
</GetCallerIdentityResponse>`, generateUUID())
}
