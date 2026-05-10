package main

// ConfigKeyMeta describes one operator-facing config key the admin
// recognises. Hot-reloadable keys can be applied without restarting
// the component; everything else needs a restart.
//
// The metadata lives admin-side, NOT on the component, per the
// components-decoupled invariant: components don't grow a "describe
// my config" endpoint. Admin owns the operator's mental model and is
// expected to drift behind component reality between releases — when
// an unrecognised key shows up in operator Config, the UI defaults to
// "restart required" (the safe default).
type ConfigKeyMeta struct {
	Name          string `json:"name"`
	HotReloadable bool   `json:"hot_reloadable"`
	Doc           string `json:"doc,omitempty"`
}

// hotReloadable lists every config key admin knows can be applied
// without restarting a component. Keys absent from the list are
// treated as restart-required.
//
// Only zero-side-effect keys belong here — log levels, feature flags
// the component re-reads from env on each request, debug toggles.
// Anything that controls binding (ports, addresses), persistence
// (data dirs, drivers), or cloud-resource layout (subnets, region) is
// restart-required because changing it mid-run produces undefined
// behaviour the operator has no way to observe.
var hotReloadable = map[string]ConfigKeyMeta{
	"SIM_LOG_LEVEL": {
		Name:          "SIM_LOG_LEVEL",
		HotReloadable: true,
		Doc:           "Log level (trace/debug/info/warn/error). Re-read on each request.",
	},
	"SOCKERLESS_LOG_LEVEL": {
		Name:          "SOCKERLESS_LOG_LEVEL",
		HotReloadable: true,
		Doc:           "Log level (trace/debug/info/warn/error). Re-read on each request.",
	},
	"SIM_PULL_POLICY": {
		Name:          "SIM_PULL_POLICY",
		HotReloadable: true,
		Doc:           "Container image pull policy (always/missing/never). Re-read at container create.",
	},
}

// restartRequired lists keys admin annotates with prose so the UI can
// surface why a change forces a restart. Absence from this map is NOT
// treated as "no annotation"; the UI just shows the bare key.
var restartRequired = map[string]ConfigKeyMeta{
	"SIM_LISTEN_ADDR": {
		Name: "SIM_LISTEN_ADDR",
		Doc:  "Bind address. Changing requires re-listening on a new socket.",
	},
	"SIM_DATA_DIR": {
		Name: "SIM_DATA_DIR",
		Doc:  "On-disk state directory. Change requires re-opening the SQLite database.",
	},
	"SIM_PERSIST": {
		Name: "SIM_PERSIST",
		Doc:  "Toggle SQLite persistence. Toggling mid-run would orphan in-memory state or open a fresh DB without prior entries.",
	},
	"SIM_RUNTIME": {
		Name: "SIM_RUNTIME",
		Doc:  "Container runtime (docker/podman/process). Selected once at startup.",
	},
	"SIM_AWS_PORT": {
		Name: "SIM_AWS_PORT",
		Doc:  "AWS sim listen port. Same constraint as SIM_LISTEN_ADDR.",
	},
	"SOCKERLESS_AWS_REGION": {
		Name: "SOCKERLESS_AWS_REGION",
		Doc:  "AWS region. Cloud resource layout assumes one region per backend.",
	},
	"SOCKERLESS_AWS_ACCOUNT_ID": {
		Name: "SOCKERLESS_AWS_ACCOUNT_ID",
		Doc:  "AWS account ID. Wired into ARNs at create time.",
	},
	"SOCKERLESS_ECS_SUBNETS": {
		Name: "SOCKERLESS_ECS_SUBNETS",
		Doc:  "Subnet list for Fargate task placement. Existing tasks pinned to old subnets stay there.",
	},
	"SOCKERLESS_ECS_SECURITY_GROUPS": {
		Name: "SOCKERLESS_ECS_SECURITY_GROUPS",
		Doc:  "SG list for Fargate tasks. Existing tasks keep their original SGs.",
	},
	"SOCKERLESS_ECS_TASK_ROLE_ARN": {
		Name: "SOCKERLESS_ECS_TASK_ROLE_ARN",
		Doc:  "Task IAM role. Baked into the task definition at register time.",
	},
	"SOCKERLESS_ECS_EXECUTION_ROLE_ARN": {
		Name: "SOCKERLESS_ECS_EXECUTION_ROLE_ARN",
		Doc:  "Execution role for ECR pulls + log shipping. Baked at register time.",
	},
	"SOCKERLESS_ENDPOINT_URL": {
		Name: "SOCKERLESS_ENDPOINT_URL",
		Doc:  "Cloud endpoint override (sim address). SDK clients are constructed once at startup.",
	},
	"SOCKERLESS_NETWORK_DISCOVERY": {
		Name: "SOCKERLESS_NETWORK_DISCOVERY",
		Doc:  "Network discovery driver. Driver selected at startup; switching mid-run would strand registered services.",
	},
	"SOCKERLESS_DNS_SEARCH_DOMAIN": {
		Name: "SOCKERLESS_DNS_SEARCH_DOMAIN",
		Doc:  "Search domain injected into containers' resolv.conf. Existing containers keep the original.",
	},
}

// Annotation returns the ConfigKeyMeta for `name`, or a default entry
// (HotReloadable=false, Name=name) when admin has no annotation. The
// default is restart-required so unknown keys err on the safe side
// (UI prompts for restart rather than no-op reload).
func Annotation(name string) ConfigKeyMeta {
	if m, ok := hotReloadable[name]; ok {
		return m
	}
	if m, ok := restartRequired[name]; ok {
		return m
	}
	return ConfigKeyMeta{Name: name, HotReloadable: false}
}

// AllAnnotations returns the curated metadata table the UI renders in
// the edit modal. Sorted by name for stable display.
func AllAnnotations() []ConfigKeyMeta {
	out := make([]ConfigKeyMeta, 0, len(hotReloadable)+len(restartRequired))
	for _, m := range hotReloadable {
		out = append(out, m)
	}
	for _, m := range restartRequired {
		out = append(out, m)
	}
	sortByName(out)
	return out
}

// ClassifyChanges categorises a config diff into hot-reloadable +
// restart-required key sets. Used by the PUT-config endpoint to tell
// the UI which action to offer (reload vs restart) after a save.
//
// added/removed/changed keys all flow through the same classification
// — a change to a restart-required key is restart-required regardless
// of whether the key was already there.
func ClassifyChanges(prev, next map[string]string) (hot, restart []string) {
	seen := make(map[string]bool, len(prev)+len(next))
	for k, v := range next {
		seen[k] = true
		if prev[k] == v {
			continue
		}
		if Annotation(k).HotReloadable {
			hot = append(hot, k)
		} else {
			restart = append(restart, k)
		}
	}
	for k := range prev {
		if seen[k] {
			continue
		}
		// Removed key.
		if Annotation(k).HotReloadable {
			hot = append(hot, k)
		} else {
			restart = append(restart, k)
		}
	}
	sortStrings(hot)
	sortStrings(restart)
	return hot, restart
}

// sortByName / sortStrings — small helpers to avoid pulling sort into
// every file. Match the existing pattern in instance_lifecycle.go's
// sortedConfigKeys.
func sortByName(s []ConfigKeyMeta) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1].Name > s[j].Name; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
