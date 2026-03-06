package lambda

import (
	"net/http"
	"time"

	core "github.com/sockerless/backend-core"
)

func (s *Server) writeLogEvent(w http.ResponseWriter, message *string, timestamp *int64, params core.CloudLogParams) {
	if message == nil {
		return
	}
	var ts time.Time
	if timestamp != nil {
		ts = time.UnixMilli(*timestamp)
	}
	line := params.FormatLine(*message, ts)
	core.WriteMuxLine(w, line)
}
