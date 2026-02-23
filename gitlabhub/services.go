package gitlabhub

// buildServiceDefs converts parsed service entries into the job response format.
func buildServiceDefs(services []ServiceEntry) []ServiceDef {
	var result []ServiceDef
	for _, svc := range services {
		alias := svc.Alias
		if alias == "" {
			// GitLab convention: service alias is image name without tag
			alias = serviceAlias(svc.Name)
		}
		result = append(result, ServiceDef{
			Name:       svc.Name,
			Alias:      alias,
			Entrypoint: svc.Entrypoint,
			Command:    svc.Command,
			Variables:  svc.Variables,
		})
	}
	return result
}

// serviceAlias extracts the default service alias from an image name.
// E.g., "postgres:15-alpine" → "postgres", "registry.example.com/my-redis" → "my-redis"
func serviceAlias(image string) string {
	// Strip tag
	name := image
	if idx := lastIndexByte(name, ':'); idx > 0 {
		name = name[:idx]
	}
	// Strip registry prefix: take last path component
	if idx := lastIndexByte(name, '/'); idx >= 0 {
		name = name[idx+1:]
	}
	return name
}

func lastIndexByte(s string, c byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == c {
			return i
		}
	}
	return -1
}
