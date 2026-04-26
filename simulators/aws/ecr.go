package main

import (
	"encoding/base64"
	"net/http"
	"time"

	sim "github.com/sockerless/simulator"
)

// ECR types

type ECRRepository struct {
	RepositoryArn  string `json:"repositoryArn"`
	RepositoryName string `json:"repositoryName"`
	RepositoryUri  string `json:"repositoryUri"`
	RegistryId     string `json:"registryId"`
	CreatedAt      int64  `json:"createdAt"`
}

type ECRImageDetail struct {
	RegistryId     string   `json:"registryId"`
	RepositoryName string   `json:"repositoryName"`
	ImageDigest    string   `json:"imageDigest"`
	ImageTags      []string `json:"imageTags"`
	ImageManifest  string   `json:"imageManifest"`
	PushedAt       int64    `json:"pushedAt"`
}

type ECRLifecyclePolicy struct {
	RegistryId          string `json:"registryId"`
	RepositoryName      string `json:"repositoryName"`
	LifecyclePolicyText string `json:"lifecyclePolicyText"`
}

// ECRPullThroughCacheRule models an ECR pull-through cache rule. The
// simulator stores these so callers (sockerless ECS backend, aws CLI,
// terraform) can register, list, and delete them just like real ECR.
// The rule is consulted when a container image URI's registry path
// starts with `<account>.dkr.ecr.<region>.amazonaws.com/<prefix>/…`
// and `<prefix>` matches a registered `EcrRepositoryPrefix`.
type ECRPullThroughCacheRule struct {
	EcrRepositoryPrefix string `json:"ecrRepositoryPrefix"`
	UpstreamRegistryUrl string `json:"upstreamRegistryUrl"`
	UpstreamRegistry    string `json:"upstreamRegistry,omitempty"`
	RegistryId          string `json:"registryId"`
	CreatedAt           int64  `json:"createdAt"`
	UpdatedAt           int64  `json:"updatedAt,omitempty"`
}

// State stores
var (
	ecrRepositories          sim.Store[ECRRepository]
	ecrImages                sim.Store[ECRImageDetail]
	ecrLifecyclePolicies     sim.Store[ECRLifecyclePolicy]
	ecrPullThroughCacheRules sim.Store[ECRPullThroughCacheRule]
)

// ecrRegistryId() returns the registry ID — same as the AWS account ID.
// Real ECR uses the caller's account; the sim defers to awsAccountID
// so a SOCKERLESS_AWS_ACCOUNT_ID override propagates through every ECR
// ARN, repository URI, and authorization-token endpoint.
func ecrRegistryId() string { return awsAccountID() }

func ecrArn(resourceType, name string) string {
	return "arn:aws:ecr:" + awsRegion() + ":" + ecrRegistryId() + ":" + resourceType + "/" + name
}

func registerECR(r *sim.AWSRouter, srv *sim.Server) {
	ecrRepositories = sim.MakeStore[ECRRepository](srv.DB(), "ecr_repositories")
	ecrImages = sim.MakeStore[ECRImageDetail](srv.DB(), "ecr_images")
	ecrLifecyclePolicies = sim.MakeStore[ECRLifecyclePolicy](srv.DB(), "ecr_lifecycle_policies")
	ecrPullThroughCacheRules = sim.MakeStore[ECRPullThroughCacheRule](srv.DB(), "ecr_pull_through_cache_rules")

	r.Register("AmazonEC2ContainerRegistry_V20150921.CreateRepository", handleECRCreateRepository)
	r.Register("AmazonEC2ContainerRegistry_V20150921.DescribeRepositories", handleECRDescribeRepositories)
	r.Register("AmazonEC2ContainerRegistry_V20150921.DeleteRepository", handleECRDeleteRepository)
	r.Register("AmazonEC2ContainerRegistry_V20150921.GetAuthorizationToken", handleECRGetAuthorizationToken)
	r.Register("AmazonEC2ContainerRegistry_V20150921.BatchGetImage", handleECRBatchGetImage)
	r.Register("AmazonEC2ContainerRegistry_V20150921.PutImage", handleECRPutImage)
	r.Register("AmazonEC2ContainerRegistry_V20150921.BatchDeleteImage", handleECRBatchDeleteImage)
	r.Register("AmazonEC2ContainerRegistry_V20150921.BatchCheckLayerAvailability", handleECRBatchCheckLayerAvailability)
	r.Register("AmazonEC2ContainerRegistry_V20150921.PutLifecyclePolicy", handleECRPutLifecyclePolicy)
	r.Register("AmazonEC2ContainerRegistry_V20150921.GetLifecyclePolicy", handleECRGetLifecyclePolicy)
	r.Register("AmazonEC2ContainerRegistry_V20150921.DeleteLifecyclePolicy", handleECRDeleteLifecyclePolicy)
	r.Register("AmazonEC2ContainerRegistry_V20150921.ListTagsForResource", handleECRListTagsForResource)
	r.Register("AmazonEC2ContainerRegistry_V20150921.TagResource", handleECRTagResource)

	// Pull-through cache rules. Used by sockerless image resolvers
	// and by terraform's aws_ecr_pull_through_cache_rule resource.
	// Backend caller builds URIs like
	// `<account>.dkr.ecr.<region>.amazonaws.com/<prefix>/<repo>:<tag>`
	// which the simulator's ResolveLocalImage recognizes as a cache
	// hit and rewrites to the upstream registry on first pull.
	r.Register("AmazonEC2ContainerRegistry_V20150921.CreatePullThroughCacheRule", handleECRCreatePullThroughCacheRule)
	r.Register("AmazonEC2ContainerRegistry_V20150921.DescribePullThroughCacheRules", handleECRDescribePullThroughCacheRules)
	r.Register("AmazonEC2ContainerRegistry_V20150921.DeletePullThroughCacheRule", handleECRDeletePullThroughCacheRule)
}

// handleECRCreatePullThroughCacheRule registers a pull-through cache
// rule. Returns PullThroughCacheRuleAlreadyExistsException if the
// prefix is already in use — matches real AWS behaviour so sockerless
// and terraform's `aws_ecr_pull_through_cache_rule` see the same errors.
func handleECRCreatePullThroughCacheRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EcrRepositoryPrefix string `json:"ecrRepositoryPrefix"`
		UpstreamRegistryUrl string `json:"upstreamRegistryUrl"`
		UpstreamRegistry    string `json:"upstreamRegistry"`
		RegistryId          string `json:"registryId"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.EcrRepositoryPrefix == "" || req.UpstreamRegistryUrl == "" {
		sim.AWSError(w, "InvalidParameterException", "ecrRepositoryPrefix and upstreamRegistryUrl are required", http.StatusBadRequest)
		return
	}
	if _, exists := ecrPullThroughCacheRules.Get(req.EcrRepositoryPrefix); exists {
		sim.AWSError(w, "PullThroughCacheRuleAlreadyExistsException",
			"A pull-through cache rule with the given prefix already exists",
			http.StatusBadRequest)
		return
	}
	now := time.Now().Unix()
	regID := req.RegistryId
	if regID == "" {
		regID = ecrRegistryId()
	}
	rule := ECRPullThroughCacheRule{
		EcrRepositoryPrefix: req.EcrRepositoryPrefix,
		UpstreamRegistryUrl: req.UpstreamRegistryUrl,
		UpstreamRegistry:    req.UpstreamRegistry,
		RegistryId:          regID,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	ecrPullThroughCacheRules.Put(req.EcrRepositoryPrefix, rule)

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"ecrRepositoryPrefix": rule.EcrRepositoryPrefix,
		"upstreamRegistryUrl": rule.UpstreamRegistryUrl,
		"upstreamRegistry":    rule.UpstreamRegistry,
		"registryId":          rule.RegistryId,
		"createdAt":           rule.CreatedAt,
	})
}

// handleECRDescribePullThroughCacheRules returns rules matching the
// requested prefixes, or all rules when the request is empty. Matches
// the real API's pagination-less response shape for the test-sized
// rule sets the simulator supports.
func handleECRDescribePullThroughCacheRules(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EcrRepositoryPrefixes []string `json:"ecrRepositoryPrefixes"`
	}
	_ = sim.ReadJSON(r, &req)

	var rules []ECRPullThroughCacheRule
	if len(req.EcrRepositoryPrefixes) == 0 {
		rules = ecrPullThroughCacheRules.List()
	} else {
		for _, p := range req.EcrRepositoryPrefixes {
			if rule, ok := ecrPullThroughCacheRules.Get(p); ok {
				rules = append(rules, rule)
			}
		}
	}
	if rules == nil {
		rules = []ECRPullThroughCacheRule{}
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"pullThroughCacheRules": rules,
	})
}

// handleECRDeletePullThroughCacheRule removes a rule. Returns
// PullThroughCacheRuleNotFoundException when the prefix isn't
// registered — matches real AWS.
func handleECRDeletePullThroughCacheRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EcrRepositoryPrefix string `json:"ecrRepositoryPrefix"`
		RegistryId          string `json:"registryId"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.EcrRepositoryPrefix == "" {
		sim.AWSError(w, "InvalidParameterException", "ecrRepositoryPrefix is required", http.StatusBadRequest)
		return
	}
	rule, ok := ecrPullThroughCacheRules.Get(req.EcrRepositoryPrefix)
	if !ok {
		sim.AWSError(w, "PullThroughCacheRuleNotFoundException",
			"The pull-through cache rule does not exist",
			http.StatusNotFound)
		return
	}
	ecrPullThroughCacheRules.Delete(req.EcrRepositoryPrefix)

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"ecrRepositoryPrefix": rule.EcrRepositoryPrefix,
		"upstreamRegistryUrl": rule.UpstreamRegistryUrl,
		"upstreamRegistry":    rule.UpstreamRegistry,
		"registryId":          rule.RegistryId,
		"createdAt":           rule.CreatedAt,
	})
}

// Pull-through-cache URI → local docker ref resolution is handled by
// `sim.ResolveLocalImage` in simulators/aws/shared/container.go, which
// already strips `docker-hub/` + `library/` prefixes from ECR URIs
// before the simulator pulls. The handlers above are what sockerless's
// image-resolver + terraform's aws_ecr_pull_through_cache_rule need;
// the launch-time URI mapping lives in the shared helper.

func handleECRCreateRepository(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RepositoryName string `json:"repositoryName"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.RepositoryName == "" {
		sim.AWSError(w, "InvalidParameterException", "repositoryName is required", http.StatusBadRequest)
		return
	}

	if _, exists := ecrRepositories.Get(req.RepositoryName); exists {
		sim.AWSErrorf(w, "RepositoryAlreadyExistsException", http.StatusBadRequest,
			"The repository with name '%s' already exists", req.RepositoryName)
		return
	}

	repo := ECRRepository{
		RepositoryArn:  ecrArn("repository", req.RepositoryName),
		RepositoryName: req.RepositoryName,
		RepositoryUri:  ecrRegistryId() + ".dkr.ecr." + awsRegion() + ".amazonaws.com/" + req.RepositoryName,
		RegistryId:     ecrRegistryId(),
		CreatedAt:      time.Now().Unix(),
	}
	ecrRepositories.Put(req.RepositoryName, repo)

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"repository": repo,
	})
}

func handleECRDescribeRepositories(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RepositoryNames []string `json:"repositoryNames"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}

	var repos []ECRRepository
	if len(req.RepositoryNames) == 0 {
		repos = ecrRepositories.List()
	} else {
		for _, name := range req.RepositoryNames {
			repo, ok := ecrRepositories.Get(name)
			if ok {
				repos = append(repos, repo)
			}
		}
	}
	if repos == nil {
		repos = []ECRRepository{}
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"repositories": repos,
	})
}

func handleECRDeleteRepository(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RepositoryName string `json:"repositoryName"`
		Force          bool   `json:"force"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.RepositoryName == "" {
		sim.AWSError(w, "InvalidParameterException", "repositoryName is required", http.StatusBadRequest)
		return
	}

	repo, ok := ecrRepositories.Get(req.RepositoryName)
	if !ok {
		sim.AWSErrorf(w, "RepositoryNotFoundException", http.StatusBadRequest,
			"The repository with name '%s' does not exist", req.RepositoryName)
		return
	}

	ecrRepositories.Delete(req.RepositoryName)

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"repository": repo,
	})
}

func handleECRGetAuthorizationToken(w http.ResponseWriter, r *http.Request) {
	// Consume request body
	_ = sim.ReadJSON(r, &struct{}{})

	token := base64.StdEncoding.EncodeToString([]byte("AWS:password"))
	expiresAt := time.Now().Add(12 * time.Hour).Unix()

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"authorizationData": []map[string]any{
			{
				"authorizationToken": token,
				"expiresAt":          expiresAt,
				"proxyEndpoint":      "https://" + ecrRegistryId() + ".dkr.ecr." + awsRegion() + ".amazonaws.com",
			},
		},
	})
}

func handleECRBatchGetImage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RepositoryName string `json:"repositoryName"`
		ImageIds       []struct {
			ImageTag    string `json:"imageTag"`
			ImageDigest string `json:"imageDigest"`
		} `json:"imageIds"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}

	var images []map[string]any
	var failures []map[string]any

	for _, imageId := range req.ImageIds {
		key := req.RepositoryName + ":" + imageId.ImageTag
		if imageId.ImageDigest != "" {
			key = req.RepositoryName + ":" + imageId.ImageDigest
		}

		img, ok := ecrImages.Get(key)
		if ok {
			images = append(images, map[string]any{
				"registryId":     img.RegistryId,
				"repositoryName": img.RepositoryName,
				"imageId": map[string]string{
					"imageDigest": img.ImageDigest,
					"imageTag":    imageId.ImageTag,
				},
				"imageManifest": img.ImageManifest,
			})
		} else {
			failures = append(failures, map[string]any{
				"imageId": map[string]string{
					"imageTag": imageId.ImageTag,
				},
				"failureCode":   "ImageNotFound",
				"failureReason": "Requested image not found",
			})
		}
	}
	if images == nil {
		images = []map[string]any{}
	}
	if failures == nil {
		failures = []map[string]any{}
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"images":   images,
		"failures": failures,
	})
}

func handleECRPutImage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RepositoryName string `json:"repositoryName"`
		ImageManifest  string `json:"imageManifest"`
		ImageTag       string `json:"imageTag"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.RepositoryName == "" || req.ImageManifest == "" {
		sim.AWSError(w, "InvalidParameterException", "repositoryName and imageManifest are required", http.StatusBadRequest)
		return
	}

	if _, ok := ecrRepositories.Get(req.RepositoryName); !ok {
		sim.AWSErrorf(w, "RepositoryNotFoundException", http.StatusBadRequest,
			"The repository with name '%s' does not exist", req.RepositoryName)
		return
	}

	digest := "sha256:" + generateUUID()
	img := ECRImageDetail{
		RegistryId:     ecrRegistryId(),
		RepositoryName: req.RepositoryName,
		ImageDigest:    digest,
		ImageTags:      []string{req.ImageTag},
		ImageManifest:  req.ImageManifest,
		PushedAt:       time.Now().Unix(),
	}

	key := req.RepositoryName + ":" + req.ImageTag
	ecrImages.Put(key, img)
	// Also store by digest
	ecrImages.Put(req.RepositoryName+":"+digest, img)

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"image": map[string]any{
			"registryId":     img.RegistryId,
			"repositoryName": img.RepositoryName,
			"imageId": map[string]string{
				"imageDigest": img.ImageDigest,
				"imageTag":    req.ImageTag,
			},
			"imageManifest": img.ImageManifest,
		},
	})
}

func handleECRBatchDeleteImage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RepositoryName string `json:"repositoryName"`
		ImageIds       []struct {
			ImageTag    string `json:"imageTag"`
			ImageDigest string `json:"imageDigest"`
		} `json:"imageIds"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}

	if _, ok := ecrRepositories.Get(req.RepositoryName); !ok {
		sim.AWSErrorf(w, "RepositoryNotFoundException", http.StatusBadRequest,
			"The repository with name '%s' does not exist", req.RepositoryName)
		return
	}

	var deleted []map[string]any
	var failures []map[string]any

	for _, imageId := range req.ImageIds {
		key := req.RepositoryName + ":" + imageId.ImageTag
		if imageId.ImageDigest != "" {
			key = req.RepositoryName + ":" + imageId.ImageDigest
		}

		if _, ok := ecrImages.Get(key); ok {
			ecrImages.Delete(key)
			imgId := map[string]string{}
			if imageId.ImageTag != "" {
				imgId["imageTag"] = imageId.ImageTag
			}
			if imageId.ImageDigest != "" {
				imgId["imageDigest"] = imageId.ImageDigest
			}
			deleted = append(deleted, map[string]any{"imageId": imgId})
		} else {
			imgId := map[string]string{}
			if imageId.ImageTag != "" {
				imgId["imageTag"] = imageId.ImageTag
			}
			if imageId.ImageDigest != "" {
				imgId["imageDigest"] = imageId.ImageDigest
			}
			failures = append(failures, map[string]any{
				"imageId":       imgId,
				"failureCode":   "ImageNotFound",
				"failureReason": "Requested image not found",
			})
		}
	}
	if deleted == nil {
		deleted = []map[string]any{}
	}
	if failures == nil {
		failures = []map[string]any{}
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"imageIds": deleted,
		"failures": failures,
	})
}

func handleECRBatchCheckLayerAvailability(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RepositoryName string   `json:"repositoryName"`
		LayerDigests   []string `json:"layerDigests"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}

	var layers []map[string]any
	for _, digest := range req.LayerDigests {
		layers = append(layers, map[string]any{
			"layerDigest":       digest,
			"layerAvailability": "AVAILABLE",
		})
	}
	if layers == nil {
		layers = []map[string]any{}
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"layers":   layers,
		"failures": []any{},
	})
}

func handleECRPutLifecyclePolicy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RepositoryName      string `json:"repositoryName"`
		LifecyclePolicyText string `json:"lifecyclePolicyText"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}

	policy := ECRLifecyclePolicy{
		RegistryId:          ecrRegistryId(),
		RepositoryName:      req.RepositoryName,
		LifecyclePolicyText: req.LifecyclePolicyText,
	}
	ecrLifecyclePolicies.Put(req.RepositoryName, policy)

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"registryId":          ecrRegistryId(),
		"repositoryName":      req.RepositoryName,
		"lifecyclePolicyText": req.LifecyclePolicyText,
	})
}

func handleECRGetLifecyclePolicy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RepositoryName string `json:"repositoryName"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}

	policy, ok := ecrLifecyclePolicies.Get(req.RepositoryName)
	if !ok {
		sim.AWSErrorf(w, "LifecyclePolicyNotFoundException", http.StatusBadRequest,
			"Lifecycle policy for repository '%s' does not exist", req.RepositoryName)
		return
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"registryId":          policy.RegistryId,
		"repositoryName":      policy.RepositoryName,
		"lifecyclePolicyText": policy.LifecyclePolicyText,
	})
}

func handleECRDeleteLifecyclePolicy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RepositoryName string `json:"repositoryName"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}

	policy, ok := ecrLifecyclePolicies.Get(req.RepositoryName)
	if !ok {
		sim.AWSErrorf(w, "LifecyclePolicyNotFoundException", http.StatusBadRequest,
			"Lifecycle policy for repository '%s' does not exist", req.RepositoryName)
		return
	}

	ecrLifecyclePolicies.Delete(req.RepositoryName)

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"registryId":          policy.RegistryId,
		"repositoryName":      policy.RepositoryName,
		"lifecyclePolicyText": policy.LifecyclePolicyText,
	})
}

func handleECRListTagsForResource(w http.ResponseWriter, r *http.Request) {
	// Terraform uses this to read tags for ECR repositories
	_ = sim.ReadJSON(r, &struct{}{})
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"tags": []any{},
	})
}

func handleECRTagResource(w http.ResponseWriter, r *http.Request) {
	// Accept and discard tag operations
	_ = sim.ReadJSON(r, &struct{}{})
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}
