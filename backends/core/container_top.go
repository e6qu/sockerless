package core

import (
	"fmt"
	"strings"

	"github.com/sockerless/api"
)

// RunContainerTopViaAgent executes `ps <psArgs>` inside the container
// over the reverse-agent WS and parses the output into the
// api.ContainerTopResponse shape docker clients expect. Used by every
// backend that has a reverse-agent path (Lambda, CR, ACA).
//
// psArgs is the argv passed to `ps`; empty means `-ef` (matching
// docker's default when `?ps_args=` isn't set). The first output line
// is the column headers, subsequent lines are the processes. The
// reverse-agent path gives backends a working `docker top` even though
// their control-plane APIs don't expose process listing.
func RunContainerTopViaAgent(reg *ReverseAgentRegistry, containerID, psArgs string) (*api.ContainerTopResponse, error) {
	if reg == nil {
		return nil, ErrNoReverseAgent
	}
	argv := []string{"ps"}
	if psArgs != "" {
		argv = append(argv, strings.Fields(psArgs)...)
	} else {
		argv = append(argv, "-ef")
	}
	stdout, stderr, exit, err := reg.RunAndCapture(containerID, "top-"+containerID, argv, nil, "")
	if err != nil {
		return nil, err
	}
	if exit != 0 {
		return nil, fmt.Errorf("ps failed with exit %d: %s", exit, strings.TrimSpace(string(stderr)))
	}
	return ParseTopOutput(string(stdout)), nil
}

// ParseTopOutput splits `ps` stdout into titles + processes. Handles
// variable-width column layouts (common `ps -ef` output) by using
// rune-aware field boundaries.
func ParseTopOutput(raw string) *api.ContainerTopResponse {
	lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
	resp := &api.ContainerTopResponse{Titles: []string{}, Processes: [][]string{}}
	if len(lines) == 0 {
		return resp
	}
	// First non-empty line is the header.
	var header string
	rest := lines
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			header = l
			rest = lines[i+1:]
			break
		}
	}
	resp.Titles = strings.Fields(header)
	nCols := len(resp.Titles)
	if nCols == 0 {
		return resp
	}
	for _, l := range rest {
		if strings.TrimSpace(l) == "" {
			continue
		}
		// Split into nCols fields — last column absorbs whitespace.
		fields := strings.Fields(l)
		if len(fields) < nCols {
			// Short row; pad with empty strings.
			for len(fields) < nCols {
				fields = append(fields, "")
			}
			resp.Processes = append(resp.Processes, fields)
			continue
		}
		// Merge trailing fields into the last column (e.g. COMMAND
		// column often contains spaces).
		row := make([]string, nCols)
		copy(row, fields[:nCols-1])
		row[nCols-1] = strings.Join(fields[nCols-1:], " ")
		resp.Processes = append(resp.Processes, row)
	}
	return resp
}
