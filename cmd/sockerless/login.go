package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// loginTokenFileName is the Shauth ID token written per context. The aws CLI
// reads it through web_identity_token_file and gcloud/google-auth through the
// external_account credential_source, so the vendor tools run the federation
// exchange themselves whenever they need cloud credentials.
const loginTokenFileName = "web-identity-token"

// gcpADCFileName is the Google Cloud workforce external_account Application
// Default Credentials file written per context.
const gcpADCFileName = "gcp-adc.json"

var loginCloudNames = []string{"aws", "gcp", "azure"}

// cloudSelection reports, for one cloud, whether the context carries the
// login coordinates for it. Skip is empty when configured; otherwise it names
// the config.yaml keys whose absence excludes the cloud from this login.
type cloudSelection struct {
	Name string
	Skip string
}

// loginCloudSelections evaluates which clouds the context configures for
// login. Absence of a cloud's federation coordinates is configuration — that
// cloud is skipped loudly, never silently.
func loginCloudSelections(env *environment, ctxName string) []cloudSelection {
	prefix := "environments." + ctxName
	selections := make([]cloudSelection, 0, len(loginCloudNames))

	awsSelection := cloudSelection{Name: "aws"}
	if env.AWS == nil || env.AWS.Login == nil || env.AWS.Login.RoleARN == "" {
		awsSelection.Skip = "set " + prefix + ".aws.login.role_arn"
	}
	selections = append(selections, awsSelection)

	gcpSelection := cloudSelection{Name: "gcp"}
	if env.GCP == nil || env.GCP.Login == nil || env.GCP.Login.WorkforceAudience == "" {
		gcpSelection.Skip = "set " + prefix + ".gcp.login.workforce_audience"
	}
	selections = append(selections, gcpSelection)

	azureSelection := cloudSelection{Name: "azure"}
	if env.Azure == nil || env.Azure.Login == nil || env.Azure.Login.Tenant == "" || env.Azure.Login.ClientID == "" {
		azureSelection.Skip = "set " + prefix + ".azure.login.tenant and " + prefix + ".azure.login.client_id"
	}
	selections = append(selections, azureSelection)

	return selections
}

// filterCloudSelections applies --cloud. An unknown cloud name is an error; a
// known but unconfigured cloud keeps its skip reason so the caller fails
// loudly instead of silently doing nothing.
func filterCloudSelections(selections []cloudSelection, only string) ([]cloudSelection, error) {
	if only == "" {
		return selections, nil
	}
	for _, selection := range selections {
		if selection.Name == only {
			return []cloudSelection{selection}, nil
		}
	}
	return nil, fmt.Errorf("unknown cloud %q (expected one of: %s)", only, strings.Join(loginCloudNames, ", "))
}

// activeLoginContext loads the active context and its login coordinates,
// exiting loudly when either is missing.
func activeLoginContext() (string, *environment) {
	name := activeContextName()
	if name == "" {
		fmt.Fprintln(os.Stderr, "error: no active context. Run `sockerless context use <name>` first.")
		os.Exit(1)
	}
	cfg := requireConfigFile()
	env, ok := cfg.Environments[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "error: active context %q not found in config.yaml\n", name)
		os.Exit(1)
	}
	return name, env
}

func contextLoginDir(ctxName string) string {
	return filepath.Join(sockerlessDir(), "contexts", ctxName)
}

func cmdLogin(args []string) {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	cloud := fs.String("cloud", "", "limit login to one cloud: aws, gcp, or azure")
	noBrowser := fs.Bool("no-browser", false, "print the sign-in URL instead of opening a browser")
	timeout := fs.Duration("timeout", 5*time.Minute, "how long to wait for the browser sign-in")
	_ = fs.Parse(args)

	ctxName, env := activeLoginContext()
	if env.Login == nil || env.Login.Issuer == "" || env.Login.ClientID == "" {
		fmt.Fprintf(os.Stderr, "error: login is not configured for context %q (set environments.%s.login.issuer and environments.%s.login.client_id in config.yaml)\n", ctxName, ctxName, ctxName)
		os.Exit(1)
	}

	selections, err := filterCloudSelections(loginCloudSelections(env, ctxName), *cloud)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	var clouds []string
	for _, selection := range selections {
		if selection.Skip != "" {
			fmt.Printf("%s: not configured (%s in config.yaml) — skipped\n", selection.Name, selection.Skip)
			continue
		}
		clouds = append(clouds, selection.Name)
	}
	if len(clouds) == 0 {
		fmt.Fprintln(os.Stderr, "error: no cloud is configured for login in this context")
		os.Exit(1)
	}

	browse := openBrowser
	if *noBrowser {
		browse = nil
	}
	idToken, err := browserLogin(loginFlowOptions{
		Issuer:       env.Login.Issuer,
		ClientID:     env.Login.ClientID,
		ClientSecret: env.Login.ClientSecret,
		Timeout:      *timeout,
		Browse:       browse,
		Printf:       func(format string, a ...any) { fmt.Printf(format, a...) },
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	claims, err := idTokenClaims(idToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	identity := stringClaim(claims, "preferred_username")
	if identity == "" {
		identity = stringClaim(claims, "sub")
	}
	fmt.Printf("Signed in as %s (subject %s)\n", identity, stringClaim(claims, "sub"))
	if expiry, ok := claims["exp"].(float64); ok {
		fmt.Printf("The identity token expires at %s; run `sockerless login` again after that.\n", time.Unix(int64(expiry), 0).Format(time.RFC3339))
	}

	tokenDir := contextLoginDir(ctxName)
	if err := os.MkdirAll(tokenDir, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	tokenPath := filepath.Join(tokenDir, loginTokenFileName)
	if err := os.WriteFile(tokenPath, []byte(idToken), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Wrote identity token to %s\n", tokenPath)

	failed := false
	for _, name := range clouds {
		fmt.Printf("\n== %s\n", name)
		var wireErr error
		switch name {
		case "aws":
			wireErr = wireAWSLogin(ctxName, env, tokenPath)
		case "gcp":
			wireErr = wireGCPLogin(ctxName, env, tokenPath)
		case "azure":
			wireErr = wireAzureLogin(ctxName, env, idToken)
		}
		if wireErr != nil {
			failed = true
			fmt.Fprintf(os.Stderr, "%s: %v\n", name, wireErr)
		}
	}
	if failed {
		os.Exit(1)
	}
}

// wireAWSLogin writes the aws CLI profile whose credentials the aws CLI and
// SDKs mint themselves: role_arn + web_identity_token_file make every
// invocation run AssumeRoleWithWebIdentity against the profile's endpoint.
func wireAWSLogin(ctxName string, env *environment, tokenPath string) error {
	login := env.AWS.Login
	region := env.AWS.Region
	if region == "" {
		return fmt.Errorf("aws login requires a region (set environments.%s.aws.region in config.yaml)", ctxName)
	}
	profile := login.Profile
	if profile == "" {
		profile = "sockerless-" + ctxName
	}
	values := []iniKV{
		{"role_arn", login.RoleARN},
		{"web_identity_token_file", tokenPath},
		{"region", region},
	}
	if login.EndpointURL != "" {
		values = append(values, iniKV{"endpoint_url", login.EndpointURL})
	}
	configPath := awsConfigFilePath()
	if err := upsertINISection(configPath, "profile "+profile, values); err != nil {
		return fmt.Errorf("update %s: %w", configPath, err)
	}
	fmt.Printf("Wrote profile [%s] to %s\n", profile, configPath)
	fmt.Printf("Verify with: aws --profile %s sts get-caller-identity\n", profile)
	return nil
}

// externalAccountCredentials is the Application Default Credentials shape for
// Google Cloud Workforce Identity Federation (an `external_account` file with
// a file-sourced subject token).
type externalAccountCredentials struct {
	Type                     string                          `json:"type"`
	Audience                 string                          `json:"audience"`
	SubjectTokenType         string                          `json:"subject_token_type"`
	TokenURL                 string                          `json:"token_url"`
	TokenInfoURL             string                          `json:"token_info_url"`
	CredentialSource         externalAccountCredentialSource `json:"credential_source"`
	WorkforcePoolUserProject string                          `json:"workforce_pool_user_project,omitempty"`
}

type externalAccountCredentialSource struct {
	File string `json:"file"`
}

// gcpExternalAccountJSON renders the workforce external_account credentials
// for a context. stsEndpoint defaults to real Google's Security Token
// Service; a deployed simulator is the same API at another coordinate.
func gcpExternalAccountJSON(login *gcpLoginConfig, project, tokenPath string) ([]byte, error) {
	stsBase := strings.TrimSuffix(login.STSEndpoint, "/")
	if stsBase == "" {
		stsBase = "https://sts.googleapis.com"
	}
	userProject := login.WorkforcePoolUserProject
	if userProject == "" {
		userProject = project
	}
	credentials := externalAccountCredentials{
		Type:                     "external_account",
		Audience:                 login.WorkforceAudience,
		SubjectTokenType:         "urn:ietf:params:oauth:token-type:id_token",
		TokenURL:                 stsBase + "/v1/token",
		TokenInfoURL:             stsBase + "/v1/introspect",
		CredentialSource:         externalAccountCredentialSource{File: tokenPath},
		WorkforcePoolUserProject: userProject,
	}
	return json.MarshalIndent(credentials, "", "  ")
}

// gcloudEndpointOverrides lists the `gcloud config set` properties that point
// the Google Cloud CLI at a simulator deployment, mirroring the settings the
// simulator CLI test suite drives through CLOUDSDK_API_ENDPOINT_OVERRIDES_*.
// The suite's api_gateway and bigtable overrides exist only as environment
// variables (read by the API Gateway surface and the cbt CLI), not as
// registered gcloud config properties, so they are not set here.
func gcloudEndpointOverrides(base string) [][2]string {
	base = strings.TrimSuffix(base, "/")
	root := base + "/"
	return [][2]string{
		{"api_endpoint_overrides/dns", root},
		{"api_endpoint_overrides/apigateway", root},
		{"api_endpoint_overrides/cloudbuild", root},
		{"api_endpoint_overrides/cloudresourcemanager", root},
		{"api_endpoint_overrides/iam", root},
		{"api_endpoint_overrides/pubsub", root},
		{"api_endpoint_overrides/logging", root},
		{"api_endpoint_overrides/cloudfunctions", root},
		{"api_endpoint_overrides/serviceusage", root},
		{"api_endpoint_overrides/secretmanager", root},
		{"api_endpoint_overrides/cloudkms", root},
		{"api_endpoint_overrides/vpcaccess", root},
		{"api_endpoint_overrides/compute", root},
		{"api_endpoint_overrides/artifactregistry", root},
		{"api_endpoint_overrides/eventarc", root},
		{"api_endpoint_overrides/storage", root},
		{"api_endpoint_overrides/bigquery", base + "/bigquery/v2/"},
		{"api_endpoint_overrides/firestore", root},
		{"api_endpoint_overrides/redis", root},
		{"api_endpoint_overrides/sql", root},
		{"api_endpoint_overrides/spanner", base + "/spanner/"},
		{"api_endpoint_overrides/dataflow", root},
		{"api_endpoint_overrides/bigtableadmin", root},
	}
}

func gcloudConfigurationName(ctxName string, login *gcpLoginConfig) string {
	if login.Configuration != "" {
		return login.Configuration
	}
	return "sockerless-" + ctxName
}

// wireGCPLogin writes the workforce Application Default Credentials file and
// activates it in a dedicated gcloud configuration via `gcloud auth login
// --cred-file`, so the Google Cloud CLI refreshes federated credentials
// itself from the token file.
func wireGCPLogin(ctxName string, env *environment, tokenPath string) error {
	login := env.GCP.Login
	adc, err := gcpExternalAccountJSON(login, env.GCP.Project, tokenPath)
	if err != nil {
		return err
	}
	adcPath := filepath.Join(contextLoginDir(ctxName), gcpADCFileName)
	if err := os.WriteFile(adcPath, append(adc, '\n'), 0o600); err != nil {
		return err
	}
	fmt.Printf("Wrote workforce Application Default Credentials to %s\n", adcPath)
	fmt.Printf("SDKs use them via: export GOOGLE_APPLICATION_CREDENTIALS=%s\n", adcPath)

	configuration := gcloudConfigurationName(ctxName, login)
	activation := [][]string{{"auth", "login", "--cred-file=" + adcPath, "--brief", "--quiet"}}
	var propertySets [][]string
	if env.GCP.Project != "" {
		propertySets = append(propertySets, []string{"config", "set", "project", env.GCP.Project})
	}
	if login.APIEndpoint != "" {
		for _, override := range gcloudEndpointOverrides(login.APIEndpoint) {
			propertySets = append(propertySets, []string{"config", "set", override[0], override[1]})
		}
	}

	gcloudPath, err := exec.LookPath("gcloud")
	if err != nil {
		var commands strings.Builder
		fmt.Fprintf(&commands, "  gcloud config configurations create %s --no-activate\n", configuration)
		for _, args := range append(propertySets, activation...) {
			fmt.Fprintf(&commands, "  CLOUDSDK_ACTIVE_CONFIG_NAME=%s gcloud %s\n", configuration, strings.Join(args, " "))
		}
		return fmt.Errorf("the gcloud CLI is not on PATH; install it and run:\n%s", commands.String())
	}

	configEnv := []string{"CLOUDSDK_ACTIVE_CONFIG_NAME=" + configuration, "CLOUDSDK_CORE_DISABLE_PROMPTS=1"}
	if err := runVendorCommand(nil, gcloudPath, "config", "configurations", "describe", configuration); err != nil {
		// --no-activate keeps the user's globally active configuration; every
		// command below selects ours through CLOUDSDK_ACTIVE_CONFIG_NAME.
		if err := runVendorCommand(nil, gcloudPath, "config", "configurations", "create", configuration, "--no-activate"); err != nil {
			return err
		}
	}
	for _, args := range append(propertySets, activation...) {
		if err := runVendorCommand(configEnv, gcloudPath, args...); err != nil {
			return err
		}
	}
	fmt.Printf("Activated gcloud configuration %q\n", configuration)
	fmt.Printf("Verify with: gcloud --configuration=%s projects list\n", configuration)
	return nil
}

// wireAzureLogin signs the az CLI in through Microsoft Entra Workload
// Identity Federation: `az login --service-principal --federated-token`
// stores the assertion in az's own token cache and az re-exchanges it itself
// until the assertion expires.
func wireAzureLogin(ctxName string, env *environment, idToken string) error {
	login := env.Azure.Login
	var azEnv []string
	if login.CABundle != "" {
		azEnv = append(azEnv, "REQUESTS_CA_BUNDLE="+login.CABundle)
	}
	loginArgs := []string{"login", "--service-principal", "--username", login.ClientID, "--tenant", login.Tenant, "--federated-token", idToken, "--allow-no-subscriptions", "--output", "none"}

	azPath, err := exec.LookPath("az")
	if err != nil {
		var commands strings.Builder
		if login.AuthorityEndpoint != "" || login.ResourceManagerEndpoint != "" {
			fmt.Fprintf(&commands, "  az cloud register -n %s --endpoint-active-directory %s --endpoint-resource-manager %s ...\n", azureCloudName(ctxName, login), login.AuthorityEndpoint, login.ResourceManagerEndpoint)
			fmt.Fprintf(&commands, "  az cloud set -n %s\n", azureCloudName(ctxName, login))
		}
		fmt.Fprintf(&commands, "  az login --service-principal --username %s --tenant %s --federated-token \"$(cat %s)\" --allow-no-subscriptions\n", login.ClientID, login.Tenant, filepath.Join(contextLoginDir(ctxName), loginTokenFileName))
		return fmt.Errorf("the az CLI is not on PATH; install it and run:\n%s", commands.String())
	}

	if login.AuthorityEndpoint != "" || login.ResourceManagerEndpoint != "" {
		if login.AuthorityEndpoint == "" || login.ResourceManagerEndpoint == "" {
			return fmt.Errorf("azure login requires both authority_endpoint and resource_manager_endpoint when either is set (environments.%s.azure.login)", ctxName)
		}
		cloudName := azureCloudName(ctxName, login)
		registerArgs := []string{
			"--endpoint-active-directory", login.AuthorityEndpoint,
			"--endpoint-resource-manager", login.ResourceManagerEndpoint,
			"--endpoint-management", login.ResourceManagerEndpoint,
			"--endpoint-active-directory-resource-id", "https://management.azure.com/",
			"--endpoint-active-directory-graph-resource-id", "https://graph.windows.net/",
		}
		verb := "register"
		if runVendorCommand(azEnv, azPath, "cloud", "show", "-n", cloudName, "--output", "none") == nil {
			verb = "update"
		}
		if err := runVendorCommand(azEnv, azPath, append([]string{"cloud", verb, "-n", cloudName}, registerArgs...)...); err != nil {
			return err
		}
		if err := runVendorCommand(azEnv, azPath, "cloud", "set", "-n", cloudName); err != nil {
			return err
		}
		// MSAL's instance discovery only knows the Azure public clouds; a
		// self-hosted authority (the simulator) is validated through its own
		// OpenID Connect metadata instead.
		if err := runVendorCommand(azEnv, azPath, "config", "set", "core.instance_discovery=false", "--only-show-errors"); err != nil {
			return err
		}
		fmt.Printf("Selected az cloud %q (Azure Resource Manager at %s)\n", cloudName, login.ResourceManagerEndpoint)
	}

	if err := runVendorCommand(azEnv, azPath, loginArgs...); err != nil {
		return err
	}
	fmt.Printf("Signed the az CLI in as federation client %s\n", login.ClientID)
	fmt.Printf("Verify with: az account show\n")
	return nil
}

func azureCloudName(ctxName string, login *azureLoginConfig) string {
	if login.CloudName != "" {
		return login.CloudName
	}
	return "sockerless-" + ctxName
}

// runVendorCommand runs a vendor CLI, surfacing its combined output on
// failure so the vendor's own diagnostics reach the user.
func runVendorCommand(extraEnv []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w\n%s", filepath.Base(name), strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func cmdLogout(args []string) {
	fs := flag.NewFlagSet("logout", flag.ExitOnError)
	_ = fs.Parse(args)

	ctxName, env := activeLoginContext()
	failed := false

	for _, name := range []string{loginTokenFileName, gcpADCFileName} {
		path := filepath.Join(contextLoginDir(ctxName), name)
		switch err := os.Remove(path); {
		case err == nil:
			fmt.Printf("Removed %s\n", path)
		case os.IsNotExist(err):
			fmt.Printf("Already absent: %s\n", path)
		default:
			failed = true
			fmt.Fprintf(os.Stderr, "error: remove %s: %v\n", path, err)
		}
	}

	profile := "sockerless-" + ctxName
	if env.AWS != nil && env.AWS.Login != nil && env.AWS.Login.Profile != "" {
		profile = env.AWS.Login.Profile
	}
	configPath := awsConfigFilePath()
	switch found, err := removeINISection(configPath, "profile "+profile); {
	case err != nil:
		failed = true
		fmt.Fprintf(os.Stderr, "error: remove profile [%s] from %s: %v\n", profile, configPath, err)
	case found:
		fmt.Printf("Removed profile [%s] from %s\n", profile, configPath)
	default:
		fmt.Printf("Already absent: profile [%s] in %s\n", profile, configPath)
	}

	if azPath, err := exec.LookPath("az"); err == nil {
		var azEnv []string
		if env.Azure != nil && env.Azure.Login != nil && env.Azure.Login.CABundle != "" {
			azEnv = append(azEnv, "REQUESTS_CA_BUNDLE="+env.Azure.Login.CABundle)
		}
		if err := runVendorCommand(azEnv, azPath, "logout"); err != nil {
			// az exits nonzero when no account is signed in — the logged-out
			// end state. Anything else is a real failure.
			if strings.Contains(err.Error(), "no active accounts") {
				fmt.Printf("az logout: no signed-in account to remove\n")
			} else {
				failed = true
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
			}
		} else {
			fmt.Printf("Signed the az CLI out\n")
		}
	} else {
		fmt.Printf("az CLI not on PATH — no az token cache to clear\n")
	}

	if failed {
		os.Exit(1)
	}
}
