package api

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// specFile is the OpenAPI spec that serves as the single source of truth.
const specFile = "openapi.yaml"

// openAPISpec is the top-level structure of the OpenAPI spec.
type openAPISpec struct {
	Components struct {
		Schemas map[string]openAPISchema `yaml:"schemas"`
	} `yaml:"components"`
}

type openAPISchema struct {
	Properties  map[string]openAPIProp `yaml:"properties"`
	GoEmbeds    []string               `yaml:"x-sockerless-go-embeds"`
	Extensions  []string               `yaml:"x-sockerless-extensions"`
	SkipCodegen bool                   `yaml:"x-sockerless-codegen-skip"`
}

type openAPIProp struct {
	Type string `yaml:"type"`
}

// goTypes maps the spec schema name to the Go reflect.Type.
var goTypes = map[string]reflect.Type{
	"Container":                reflect.TypeOf(Container{}),
	"ContainerState":           reflect.TypeOf(ContainerState{}),
	"ContainerConfig":          reflect.TypeOf(ContainerConfig{}),
	"HostConfig":               reflect.TypeOf(HostConfig{}),
	"PortBinding":              reflect.TypeOf(PortBinding{}),
	"RestartPolicy":            reflect.TypeOf(RestartPolicy{}),
	"LogConfig":                reflect.TypeOf(LogConfig{}),
	"Mount":                    reflect.TypeOf(Mount{}),
	"BindOptions":              reflect.TypeOf(BindOptions{}),
	"VolumeOptions":            reflect.TypeOf(VolumeOptions{}),
	"VolumeDriverConfig":       reflect.TypeOf(VolumeDriverConfig{}),
	"TmpfsOptions":             reflect.TypeOf(TmpfsOptions{}),
	"NetworkSettings":          reflect.TypeOf(NetworkSettings{}),
	"EndpointSettings":         reflect.TypeOf(EndpointSettings{}),
	"EndpointIPAMConfig":       reflect.TypeOf(EndpointIPAMConfig{}),
	"MountPoint":               reflect.TypeOf(MountPoint{}),
	"ContainerSummary":         reflect.TypeOf(ContainerSummary{}),
	"Port":                     reflect.TypeOf(Port{}),
	"HostConfigSummary":        reflect.TypeOf(HostConfigSummary{}),
	"SummaryNetworkSettings":   reflect.TypeOf(SummaryNetworkSettings{}),
	"ContainerCreateRequest":   reflect.TypeOf(ContainerCreateRequest{}),
	"NetworkingConfig":         reflect.TypeOf(NetworkingConfig{}),
	"ContainerCreateResponse":  reflect.TypeOf(ContainerCreateResponse{}),
	"ContainerWaitResponse":    reflect.TypeOf(ContainerWaitResponse{}),
	"WaitError":                reflect.TypeOf(WaitError{}),
	"ContainerAttachOptions":   reflect.TypeOf(ContainerAttachOptions{}),
	"ContainerLogsOptions":     reflect.TypeOf(ContainerLogsOptions{}),
	"ContainerListOptions":     reflect.TypeOf(ContainerListOptions{}),
	"ContainerTopResponse":     reflect.TypeOf(ContainerTopResponse{}),
	"ContainerPruneResponse":   reflect.TypeOf(ContainerPruneResponse{}),
	"ContainerUpdateRequest":   reflect.TypeOf(ContainerUpdateRequest{}),
	"ContainerUpdateResponse":  reflect.TypeOf(ContainerUpdateResponse{}),
	"ContainerChangeItem":      reflect.TypeOf(ContainerChangeItem{}),
	"ContainerCommitResponse":  reflect.TypeOf(ContainerCommitResponse{}),
	"ContainerCommitRequest":   reflect.TypeOf(ContainerCommitRequest{}),
	"ContainerPathStat":        reflect.TypeOf(ContainerPathStat{}),
	"ExecCreateRequest":        reflect.TypeOf(ExecCreateRequest{}),
	"ExecCreateResponse":       reflect.TypeOf(ExecCreateResponse{}),
	"ExecStartRequest":         reflect.TypeOf(ExecStartRequest{}),
	"ExecInstance":             reflect.TypeOf(ExecInstance{}),
	"ExecProcessConfig":        reflect.TypeOf(ExecProcessConfig{}),
	"Image":                    reflect.TypeOf(Image{}),
	"GraphDriverData":          reflect.TypeOf(GraphDriverData{}),
	"RootFS":                   reflect.TypeOf(RootFS{}),
	"ImageMetadata":            reflect.TypeOf(ImageMetadata{}),
	"ImageSummary":             reflect.TypeOf(ImageSummary{}),
	"ImageDeleteResponse":      reflect.TypeOf(ImageDeleteResponse{}),
	"ImageHistoryEntry":        reflect.TypeOf(ImageHistoryEntry{}),
	"ImagePruneResponse":       reflect.TypeOf(ImagePruneResponse{}),
	"ImageListOptions":         reflect.TypeOf(ImageListOptions{}),
	"ImagePullRequest":         reflect.TypeOf(ImagePullRequest{}),
	"ImageBuildOptions":        reflect.TypeOf(ImageBuildOptions{}),
	"ImageSearchResult":        reflect.TypeOf(ImageSearchResult{}),
	"Network":                  reflect.TypeOf(Network{}),
	"IPAM":                     reflect.TypeOf(IPAM{}),
	"IPAMConfig":               reflect.TypeOf(IPAMConfig{}),
	"EndpointResource":         reflect.TypeOf(EndpointResource{}),
	"NetworkCreateRequest":     reflect.TypeOf(NetworkCreateRequest{}),
	"NetworkCreateResponse":    reflect.TypeOf(NetworkCreateResponse{}),
	"NetworkConnectRequest":    reflect.TypeOf(NetworkConnectRequest{}),
	"NetworkDisconnectRequest": reflect.TypeOf(NetworkDisconnectRequest{}),
	"NetworkPruneResponse":     reflect.TypeOf(NetworkPruneResponse{}),
	"Volume":                   reflect.TypeOf(Volume{}),
	"VolumeUsageData":          reflect.TypeOf(VolumeUsageData{}),
	"VolumeCreateRequest":      reflect.TypeOf(VolumeCreateRequest{}),
	"VolumeListResponse":       reflect.TypeOf(VolumeListResponse{}),
	"VolumePruneResponse":      reflect.TypeOf(VolumePruneResponse{}),
	"AuthRequest":              reflect.TypeOf(AuthRequest{}),
	"AuthResponse":             reflect.TypeOf(AuthResponse{}),
	"BackendInfo":              reflect.TypeOf(BackendInfo{}),
	"HealthcheckConfig":        reflect.TypeOf(HealthcheckConfig{}),
	"HealthState":              reflect.TypeOf(HealthState{}),
	"HealthLog":                reflect.TypeOf(HealthLog{}),
	"EventsOptions":            reflect.TypeOf(EventsOptions{}),
	"Event":                    reflect.TypeOf(Event{}),
	"EventActor":               reflect.TypeOf(EventActor{}),
	"DiskUsageResponse":        reflect.TypeOf(DiskUsageResponse{}),
	"BuildCache":               reflect.TypeOf(BuildCache{}),
	"PodCreateRequest":         reflect.TypeOf(PodCreateRequest{}),
	"PodCreateResponse":        reflect.TypeOf(PodCreateResponse{}),
	"PodInspectResponse":       reflect.TypeOf(PodInspectResponse{}),
	"PodInfraConfig":           reflect.TypeOf(PodInfraConfig{}),
	"PodBlkioDeviceRate":       reflect.TypeOf(PodBlkioDeviceRate{}),
	"PodInspectMount":          reflect.TypeOf(PodInspectMount{}),
	"PodInspectDevice":         reflect.TypeOf(PodInspectDevice{}),
	"PodContainerInfo":         reflect.TypeOf(PodContainerInfo{}),
	"PodListEntry":             reflect.TypeOf(PodListEntry{}),
	"PodActionResponse":        reflect.TypeOf(PodActionResponse{}),
	"PodListOptions":           reflect.TypeOf(PodListOptions{}),
}

// jsonTagName extracts the JSON field name from a struct field tag.
func jsonTagName(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "" || tag == "-" {
		return ""
	}
	name, _, _ := strings.Cut(tag, ",")
	return name
}

// collectJSONFields returns a map of JSON tag name → Go field name for a struct type,
// expanding embedded structs.
func collectJSONFields(t reflect.Type) map[string]string {
	fields := make(map[string]string)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous {
			ft := f.Type
			if ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				for k, v := range collectJSONFields(ft) {
					fields[k] = v
				}
			}
			continue
		}
		jn := jsonTagName(f)
		if jn != "" {
			fields[jn] = f.Name
		}
	}
	return fields
}

func TestFieldCoverage(t *testing.T) {
	data, err := os.ReadFile(specFile)
	if err != nil {
		t.Fatalf("failed to read spec file: %v", err)
	}

	var spec openAPISpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("failed to parse spec YAML: %v", err)
	}

	for typeName, schema := range spec.Components.Schemas {
		if schema.SkipCodegen {
			continue
		}
		t.Run(typeName, func(t *testing.T) {
			goType, ok := goTypes[typeName]
			if !ok {
				t.Fatalf("spec type %q has no Go type mapping", typeName)
			}

			goFields := collectJSONFields(goType)

			// Build extension set for known Sockerless-only fields.
			extensions := make(map[string]bool)
			for _, ext := range schema.Extensions {
				extensions[ext] = true
			}

			// Collect all spec fields including those from embedded types.
			allSpecProps := make(map[string]bool)
			for propName := range schema.Properties {
				allSpecProps[propName] = true
			}
			for _, embedName := range schema.GoEmbeds {
				if embedSchema, ok := spec.Components.Schemas[embedName]; ok {
					for propName := range embedSchema.Properties {
						allSpecProps[propName] = true
					}
				}
			}

			// Check: every spec property must exist in Go struct.
			for propName := range allSpecProps {
				if _, ok := goFields[propName]; !ok {
					t.Errorf("spec property %q missing from Go type %s", propName, typeName)
				}
			}

			// Check: every Go field must exist in spec (or be a known extension).
			for jsonName, goName := range goFields {
				if !allSpecProps[jsonName] {
					if extensions[goName] {
						continue
					}
					t.Errorf("Go field %s.%s (JSON: %q) not in spec — add to %s or mark as extension",
						typeName, goName, jsonName, specFile)
				}
			}
		})
	}

	// Check: every Go type in goTypes has a spec entry.
	for typeName := range goTypes {
		if _, ok := spec.Components.Schemas[typeName]; !ok {
			t.Errorf("Go type %q has no spec entry — add to %s", typeName, specFile)
		}
	}

	// Check: every spec type has a Go type (except codegen-skip).
	for typeName, schema := range spec.Components.Schemas {
		if schema.SkipCodegen {
			continue
		}
		if _, ok := goTypes[typeName]; !ok {
			t.Errorf("spec type %q has no Go type mapping — add to goTypes", typeName)
		}
	}
}
