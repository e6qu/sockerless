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
	RegistryId     string `json:"registryId"`
	RepositoryName string `json:"repositoryName"`
	LifecyclePolicyText string `json:"lifecyclePolicyText"`
}

// State stores
var (
	ecrRepositories     *sim.StateStore[ECRRepository]
	ecrImages           *sim.StateStore[ECRImageDetail]
	ecrLifecyclePolicies *sim.StateStore[ECRLifecyclePolicy]
)

const ecrRegistryId = "123456789012"

func ecrArn(resourceType, name string) string {
	return "arn:aws:ecr:us-east-1:" + ecrRegistryId + ":" + resourceType + "/" + name
}

func registerECR(r *sim.AWSRouter, srv *sim.Server) {
	ecrRepositories = sim.NewStateStore[ECRRepository]()
	ecrImages = sim.NewStateStore[ECRImageDetail]()
	ecrLifecyclePolicies = sim.NewStateStore[ECRLifecyclePolicy]()

	r.Register("AmazonEC2ContainerRegistry_V20150921.CreateRepository", handleECRCreateRepository)
	r.Register("AmazonEC2ContainerRegistry_V20150921.DescribeRepositories", handleECRDescribeRepositories)
	r.Register("AmazonEC2ContainerRegistry_V20150921.DeleteRepository", handleECRDeleteRepository)
	r.Register("AmazonEC2ContainerRegistry_V20150921.GetAuthorizationToken", handleECRGetAuthorizationToken)
	r.Register("AmazonEC2ContainerRegistry_V20150921.BatchGetImage", handleECRBatchGetImage)
	r.Register("AmazonEC2ContainerRegistry_V20150921.PutImage", handleECRPutImage)
	r.Register("AmazonEC2ContainerRegistry_V20150921.BatchCheckLayerAvailability", handleECRBatchCheckLayerAvailability)
	r.Register("AmazonEC2ContainerRegistry_V20150921.PutLifecyclePolicy", handleECRPutLifecyclePolicy)
	r.Register("AmazonEC2ContainerRegistry_V20150921.GetLifecyclePolicy", handleECRGetLifecyclePolicy)
	r.Register("AmazonEC2ContainerRegistry_V20150921.DeleteLifecyclePolicy", handleECRDeleteLifecyclePolicy)
	r.Register("AmazonEC2ContainerRegistry_V20150921.ListTagsForResource", handleECRListTagsForResource)
	r.Register("AmazonEC2ContainerRegistry_V20150921.TagResource", handleECRTagResource)
}

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
		RepositoryUri:  ecrRegistryId + ".dkr.ecr.us-east-1.amazonaws.com/" + req.RepositoryName,
		RegistryId:     ecrRegistryId,
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
				"proxyEndpoint":      "https://" + ecrRegistryId + ".dkr.ecr.us-east-1.amazonaws.com",
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
		RegistryId:     ecrRegistryId,
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

func handleECRBatchCheckLayerAvailability(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RepositoryName string `json:"repositoryName"`
		LayerDigests   []string `json:"layerDigests"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}

	var layers []map[string]any
	for _, digest := range req.LayerDigests {
		layers = append(layers, map[string]any{
			"layerDigest":   digest,
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
		RegistryId:          ecrRegistryId,
		RepositoryName:      req.RepositoryName,
		LifecyclePolicyText: req.LifecyclePolicyText,
	}
	ecrLifecyclePolicies.Put(req.RepositoryName, policy)

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"registryId":          ecrRegistryId,
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
