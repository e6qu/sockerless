package core

import (
	"fmt"
	"strings"
)

// FormatExtraHostsEnv joins extra hosts as a comma-separated string
// suitable for the SOCKERLESS_EXTRA_HOSTS environment variable.
// Each entry is in "host:ip" format.
func FormatExtraHostsEnv(extraHosts []string) string {
	return strings.Join(extraHosts, ",")
}

// BuildHostsFile generates /etc/hosts content with standard localhost entries,
// an optional hostname entry, and any extra host entries.
func BuildHostsFile(hostname string, extraHosts []string) []byte {
	var b strings.Builder
	b.WriteString("127.0.0.1\tlocalhost\n")
	b.WriteString("::1\tlocalhost\n")
	if hostname != "" {
		fmt.Fprintf(&b, "127.0.0.1\t%s\n", hostname)
	}
	for _, entry := range extraHosts {
		parts := strings.SplitN(entry, ":", 2)
		if len(parts) == 2 {
			fmt.Fprintf(&b, "%s\t%s\n", parts[1], parts[0])
		}
	}
	return []byte(b.String())
}
