package gcf

import (
	"net/http"
	"time"
)

var gcfHTTPClient = &http.Client{Timeout: 10 * time.Minute}
