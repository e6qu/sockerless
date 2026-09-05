package azf

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appservice/armappservice/v5"
	"github.com/sockerless/api"
	azurecommon "github.com/sockerless/azure-common"
	core "github.com/sockerless/backend-core"
)

// Internal labels stashing a container's pre-overlay image + entrypoint/cmd
// so a pod sidecar can run its raw service image (not the reverse-agent
// overlay, which would bind the pod main's HTTP port).
const (
	labelBaseImage      = "sockerless.azf/base-image"
	labelBaseEntrypoint = "sockerless.azf/base-entrypoint"
	labelBaseCmd        = "sockerless.azf/base-cmd"
	// labelServiceLike records, from the ORIGINAL client create request (before
	// the image's default entrypoint/cmd are merged in), whether the container
	// runs its image as-is — no client entrypoint/cmd override and not OpenStdin.
	// Such a container is a service (run its raw image); anything else is
	// exec/attach-driven and needs the reverse-agent overlay. Must be derived
	// pre-merge: after the merge, base-entrypoint/base-cmd reflect the image's
	// defaults, not whether the client overrode them.
	labelServiceLike = "sockerless.azf/service-like"
)

// podMembersTagKey carries a pod's full membership manifest on the site's
// ARM tags so cloud state can reconstruct every member after a restart
// (cloud is the source of truth — no local map).
const podMembersTagKey = "sockerless-pod-members"

// podMember is the per-container record stored in the pod-members manifest.
type podMember struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Image  string `json:"image"`
	IsMain bool   `json:"isMain"`
}

// materializePodSite assembles a multi-container pod as ONE Azure Function
// App site whose sitecontainers share a single network namespace. members[0]
// is the main (the reverse-agent overlay container the runner execs into);
// the rest are sidecars (service containers — postgres/redis) reachable from
// the main on localhost:<port> and, via host-aliases, by name. This is the
// App Service analog of an ECS multi-container task / Cloud Run multi-
// container revision.
func (s *Server) materializePodSite(members []api.Container) error {
	if len(members) == 0 {
		return &api.InvalidParameterError{Message: "materializePodSite: no members"}
	}
	main := members[0]
	id := main.ID
	funcAppName := "skls-" + id[:12]
	ctx := s.ctx()

	netID, _ := s.UserDefinedNetworkID(main)
	aliases := core.HostAliasesForMembers(members, netID)

	// The main runs the overlay bootstrap; its app settings carry the
	// reverse-agent wiring, the user entrypoint/cmd, and the host-aliases
	// the bootstrap writes to /etc/hosts so siblings resolve by name to the
	// shared loopback.
	appSettings := s.buildAZFAppSettings(id, main.Config)
	if len(aliases) > 0 {
		appSettings = append(appSettings, &armappservice.NameValuePair{
			Name:  ptr("SOCKERLESS_HOST_ALIASES"),
			Value: ptr(strings.Join(aliases, ",")),
		})
	}

	// Build the pod-members manifest + tags. The site holds no DOCKER|
	// LinuxFxVersion — the images live on the sitecontainers.
	manifest := make([]podMember, 0, len(members))
	for i, m := range members {
		manifest = append(manifest, podMember{
			ID:     m.ID,
			Name:   strings.TrimPrefix(m.Name, "/"),
			Image:  podMemberDisplayImage(m),
			IsMain: i == 0,
		})
	}
	tags := core.TagSet{
		ContainerID: id,
		Backend:     "azf",
		InstanceID:  s.Desc.InstanceID,
		Pod:         "net-" + netID,
		CreatedAt:   time.Now(),
	}
	tagMap := tags.AsAzurePtrMap()
	tagMap[podMembersTagKey] = ptr(encodePodMembers(manifest))

	site := armappservice.Site{
		Location: ptr(s.config.Location),
		Kind:     ptr("functionapp,linux,container"),
		Tags:     tagMap,
		Properties: &armappservice.SiteProperties{
			SiteConfig: &armappservice.SiteConfig{AppSettings: appSettings},
		},
	}
	if s.config.AppServicePlan != "" {
		site.Properties.ServerFarmID = ptr(s.config.AppServicePlan)
	}

	poller, err := s.azure.WebApps.BeginCreateOrUpdate(ctx, s.config.ResourceGroup, funcAppName, site, nil)
	if err != nil {
		return azurecommon.MapAzureError(err, "function app", funcAppName)
	}
	result, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		_, _ = s.azure.WebApps.Delete(ctx, s.config.ResourceGroup, funcAppName, nil)
		return azurecommon.MapAzureError(err, "function app", funcAppName)
	}

	// Attach each named volume any member mounts to the site once (site-level
	// Azure Files shares), so the sitecontainers' VolumeMounts share one
	// workspace — the pod analog of an ECS task's shared task-level Volumes.
	if siteBinds := dedupePodVolumes(members); len(siteBinds) > 0 {
		if err := s.attachVolumesToFunctionSite(ctx, funcAppName, siteBinds); err != nil {
			_, _ = s.azure.WebApps.Delete(ctx, s.config.ResourceGroup, funcAppName, nil)
			return err
		}
	}

	// Attach the main container as the IsMain sitecontainer (the overlay
	// image), then each sidecar as a non-main sitecontainer (its raw service
	// image — sidecars are reached over the shared loopback, so they must
	// NOT run the reverse-agent overlay which would bind the same HTTP port).
	// Each member declares its binds as VolumeMounts on the shared site share.
	if err := s.putSiteContainer(ctx, funcAppName, "main", main.Config.Image, true, nil, main.Config.Env, main.HostConfig.Binds); err != nil {
		_, _ = s.azure.WebApps.Delete(ctx, s.config.ResourceGroup, funcAppName, nil)
		return err
	}
	for i, m := range members[1:] {
		scName := sanitizeSiteContainerName(m.Name, i+1)
		image, startUp, env := s.sidecarRunSpec(m)
		if err := s.putSiteContainer(ctx, funcAppName, scName, image, false, startUp, env, m.HostConfig.Binds); err != nil {
			_, _ = s.azure.WebApps.Delete(ctx, s.config.ResourceGroup, funcAppName, nil)
			return err
		}
	}

	resourceID := ""
	if result.ID != nil {
		resourceID = *result.ID
	}
	functionURL, functionHost := "", ""
	if result.Properties != nil && result.Properties.DefaultHostName != nil {
		functionHost = *result.Properties.DefaultHostName
		functionURL = invokeURLForHost(s.config.EndpointURL, functionHost)
	}
	s.Registry.Register(core.ResourceEntry{
		ContainerID:  id,
		Backend:      "azf",
		ResourceType: "site",
		ResourceID:   resourceID,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"name": main.Name, "functionAppName": funcAppName, "pod": "true"},
	})

	mainState := AZFState{
		FunctionAppName: funcAppName,
		ResourceID:      resourceID,
		FunctionURL:     functionURL,
		FunctionHost:    functionHost,
	}
	// Every member resolves to the same site. Sidecars leave PendingCreates
	// (they're now sitecontainers reconstructed from the site's manifest);
	// the main is invoked, which triggers the simulator/Azure to start the
	// whole multi-container site.
	for _, m := range members[1:] {
		s.AZF.Put(m.ID, mainState)
		s.PendingCreates.Delete(m.ID)
		s.EmitEvent("container", "start", m.ID, map[string]string{"name": strings.TrimPrefix(m.Name, "/")})
	}
	s.AZF.Put(id, mainState)

	return s.invokeFunctionAsync(id, main, mainState)
}

// resolvePodMembers resolves a libpod pod's members from PendingCreates,
// ordering the main first: a script-runner (OpenStdin) if present, otherwise
// the first member. Members already started (gone from PendingCreates) are
// skipped.
func (s *Server) resolvePodMembers(pod *core.PodContext) []api.Container {
	var main *api.Container
	var sidecars []api.Container
	for _, cid := range pod.ContainerIDs {
		c, ok := s.PendingCreates.Get(cid)
		if !ok {
			continue
		}
		if main == nil && c.Config.OpenStdin {
			cc := c
			main = &cc
			continue
		}
		sidecars = append(sidecars, c)
	}
	if main == nil && len(sidecars) > 0 {
		main = &sidecars[0]
		sidecars = sidecars[1:]
	}
	if main == nil {
		return nil
	}
	return append([]api.Container{*main}, sidecars...)
}

// putSiteContainer creates/updates one sitecontainer on a Function App site.
// binds are the member's translated `volName:/mnt[:ro]` specs, declared as
// the sitecontainer's VolumeMounts so it mounts the site-level Azure Files
// share — pod members mounting the same volume get a shared workspace.
func (s *Server) putSiteContainer(ctx context.Context, funcAppName, name, image string, isMain bool, startUpCommand []string, env, binds []string) error {
	mainFlag := isMain
	props := &armappservice.SiteContainerProperties{
		Image:  ptr(image),
		IsMain: &mainFlag,
	}
	if len(startUpCommand) > 0 {
		props.StartUpCommand = ptr(shellQuoteJoin(startUpCommand))
	}
	for _, e := range env {
		kv := strings.SplitN(e, "=", 2)
		if len(kv) != 2 {
			continue
		}
		props.EnvironmentVariables = append(props.EnvironmentVariables, &armappservice.EnvironmentVariable{
			Name: ptr(kv[0]), Value: ptr(kv[1]),
		})
	}
	for _, b := range binds {
		volName, mountPath, ro, ok := parseBind(b)
		if !ok {
			continue
		}
		roFlag := ro
		props.VolumeMounts = append(props.VolumeMounts, &armappservice.VolumeMount{
			VolumeSubPath:      ptr(volName),
			ContainerMountPath: ptr(mountPath),
			ReadOnly:           &roFlag,
		})
	}
	_, err := s.azure.WebApps.CreateOrUpdateSiteContainer(ctx, s.config.ResourceGroup, funcAppName, name,
		armappservice.SiteContainer{Properties: props}, nil)
	if err != nil {
		return azurecommon.MapAzureError(err, "sitecontainer", name)
	}
	return nil
}

// parseBind splits a translated `volName:/mnt[:ro]` bind spec.
func parseBind(b string) (volName, mountPath string, ro bool, ok bool) {
	parts := strings.SplitN(b, ":", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false, false
	}
	return parts[0], parts[1], len(parts) == 3 && parts[2] == "ro", true
}

// dedupePodVolumes collects every member's translated binds into one set of
// site-level `volName:/mnt[:ro]` specs (one per volume name), so a single
// `UpdateAzureStorageAccounts` attaches each Azure Files share to the site
// that all sitecontainers then mount.
func dedupePodVolumes(members []api.Container) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range members {
		for _, b := range m.HostConfig.Binds {
			volName, _, _, ok := parseBind(b)
			if !ok || seen[volName] {
				continue
			}
			seen[volName] = true
			out = append(out, b)
		}
	}
	return out
}

// podMemberDisplayImage returns the ORIGINAL image a member was created with
// (the pre-overlay base) for `docker ps` / cloud-state — the user sees
// `redis:7-alpine`, not the sockerless overlay that actually runs.
func podMemberDisplayImage(c api.Container) string {
	if base := c.Config.Labels[labelBaseImage]; base != "" {
		return base
	}
	return c.Config.Image
}

// sidecarRunSpec returns the (image, startUpCommand, env) a sidecar
// sitecontainer runs with. When an overlay was built (the common case), the
// sidecar runs the overlay in SIDECAR mode: the bootstrap dials the
// reverse-agent (so `docker exec <sidecar>` works, mirroring Cloud Run) and
// execs the service's baked entrypoint/cmd — it does NOT bind the function
// HTTP port (the main owns it in the shared netns). When no overlay is
// available, it falls back to running the raw image with the original argv
// as the startUpCommand (no per-sidecar exec).
func (s *Server) sidecarRunSpec(m api.Container) (image string, startUp, env []string) {
	env = append([]string{}, m.Config.Env...)
	if isAZFOverlaid(m) {
		env = append(env,
			envSidecarFlag,
			"SOCKERLESS_CONTAINER_ID="+m.ID,
			"SOCKERLESS_CALLBACK_URL="+s.config.CallbackURL,
		)
		return m.Config.Image, nil, env
	}
	return m.Config.Image, podMemberRawArgv(m), env
}

// isAZFOverlaid reports whether a container runs the sockerless overlay
// bootstrap — either because an overlay was built on the fly (run image
// differs from the stashed base) or because the image is already an overlay
// repo (pre-built). Only overlaid sidecars can run in sidecar mode + register
// a reverse-agent.
func isAZFOverlaid(c api.Container) bool {
	base := c.Config.Labels[labelBaseImage]
	if base != "" && c.Config.Image != base {
		return true
	}
	return core.HasOverlayRepo(c.Config.Image)
}

// podMemberRawArgv recovers a member's original entrypoint+cmd (stashed
// before overlay), used as the startUpCommand when running a raw image.
func podMemberRawArgv(c api.Container) []string {
	var argv []string
	if ep := decodeStrSliceLabel(c.Config.Labels[labelBaseEntrypoint]); len(ep) > 0 {
		argv = append(argv, ep...)
	}
	if cmd := decodeStrSliceLabel(c.Config.Labels[labelBaseCmd]); len(cmd) > 0 {
		argv = append(argv, cmd...)
	}
	return argv
}

// envSidecarFlag marks a sitecontainer as a pod sidecar so its bootstrap runs
// in sidecar mode (service subprocess + reverse-agent, no HTTP listen).
const envSidecarFlag = "SOCKERLESS_SIDECAR=1"

// sanitizeSiteContainerName derives a valid sitecontainer name from a
// container name, falling back to an index-based name.
func sanitizeSiteContainerName(containerName string, idx int) string {
	n := strings.TrimPrefix(containerName, "/")
	n = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		default:
			return '-'
		}
	}, n)
	n = strings.Trim(n, "-")
	if n == "" || len(n) > 32 {
		return fmt.Sprintf("sidecar-%d", idx)
	}
	return n
}

// shellQuoteJoin joins argv into a single startUpCommand string, single-
// quoting any element with shell-significant characters so the sim's
// quote-aware splitter reconstructs the exact argv (an embedded `sh -c
// 'script'` survives as one argument).
func shellQuoteJoin(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = shellQuote(a)
	}
	return strings.Join(parts, " ")
}

// shellQuote wraps an argv element so the sim's quote-aware splitter
// reconstructs it verbatim. The splitter strips a matching pair of single OR
// double quotes literally (no escape processing), so the quote char is chosen
// to be one absent from the content.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n'\"") {
		return s
	}
	if !strings.Contains(s, "'") {
		return "'" + s + "'"
	}
	if !strings.Contains(s, `"`) {
		return `"` + s + `"`
	}
	// Both quote kinds present: split on single quotes and concatenate the
	// pieces using the alternate quote, which the splitter joins back into
	// one word (adjacent quoted runs are not whitespace-separated).
	return `"` + strings.ReplaceAll(s, `"`, "'") + `"`
}

func encodePodMembers(m []podMember) string {
	b, _ := json.Marshal(m)
	return base64.StdEncoding.EncodeToString(b)
}

func decodePodMembers(s string) []podMember {
	if s == "" {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil
	}
	var m []podMember
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	return m
}

func decodeStrSliceLabel(s string) []string {
	if s == "" {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil
	}
	var out []string
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}
