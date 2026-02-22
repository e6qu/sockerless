package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// EFS types

type EFSFileSystem struct {
	FileSystemId     string `json:"FileSystemId"`
	FileSystemArn    string `json:"FileSystemArn"`
	CreationTime     int64  `json:"CreationTime"`
	LifeCycleState   string `json:"LifeCycleState"`
	Name             string `json:"Name,omitempty"`
	OwnerId          string `json:"OwnerId"`
	PerformanceMode  string `json:"PerformanceMode"`
	ThroughputMode   string `json:"ThroughputMode"`
	NumberOfMountTargets int `json:"NumberOfMountTargets"`
	SizeInBytes      struct {
		Value int64 `json:"Value"`
	} `json:"SizeInBytes"`
	Tags []EFSTag `json:"Tags,omitempty"`
}

type EFSTag struct {
	Key   string `json:"Key"`
	Value string `json:"Value"`
}

type EFSMountTarget struct {
	MountTargetId        string   `json:"MountTargetId"`
	FileSystemId         string   `json:"FileSystemId"`
	SubnetId             string   `json:"SubnetId"`
	IpAddress            string   `json:"IpAddress"`
	LifeCycleState       string   `json:"LifeCycleState"`
	NetworkInterfaceId   string   `json:"NetworkInterfaceId"`
	AvailabilityZoneId   string   `json:"AvailabilityZoneId"`
	AvailabilityZoneName string   `json:"AvailabilityZoneName"`
	OwnerId              string   `json:"OwnerId"`
	SecurityGroups       []string `json:"-"`
}

type EFSAccessPoint struct {
	AccessPointId    string            `json:"AccessPointId"`
	AccessPointArn   string            `json:"AccessPointArn"`
	FileSystemId     string            `json:"FileSystemId"`
	LifeCycleState   string            `json:"LifeCycleState"`
	Name             string            `json:"Name,omitempty"`
	OwnerId          string            `json:"OwnerId"`
	RootDirectory    *EFSRootDirectory `json:"RootDirectory,omitempty"`
	PosixUser        *EFSPosixUser     `json:"PosixUser,omitempty"`
	Tags             []EFSTag          `json:"Tags,omitempty"`
}

type EFSRootDirectory struct {
	Path         string              `json:"Path,omitempty"`
	CreationInfo *EFSCreationInfo    `json:"CreationInfo,omitempty"`
}

type EFSCreationInfo struct {
	OwnerGid    int64  `json:"OwnerGid"`
	OwnerUid    int64  `json:"OwnerUid"`
	Permissions string `json:"Permissions"`
}

type EFSPosixUser struct {
	Gid            int64   `json:"Gid"`
	Uid            int64   `json:"Uid"`
	SecondaryGids  []int64 `json:"SecondaryGids,omitempty"`
}

type EFSLifecyclePolicy struct {
	TransitionToIA                    string `json:"TransitionToIA,omitempty"`
	TransitionToPrimaryStorageClass   string `json:"TransitionToPrimaryStorageClass,omitempty"`
	TransitionToArchive               string `json:"TransitionToArchive,omitempty"`
}

// State stores
var (
	efsFileSystems       *sim.StateStore[EFSFileSystem]
	efsMountTargets      *sim.StateStore[EFSMountTarget]
	efsAccessPoints      *sim.StateStore[EFSAccessPoint]
	efsLifecyclePolicies *sim.StateStore[[]EFSLifecyclePolicy]
)

func efsArn(resourceType, id string) string {
	return fmt.Sprintf("arn:aws:elasticfilesystem:us-east-1:123456789012:%s/%s", resourceType, id)
}

func registerEFS(srv *sim.Server) {
	efsFileSystems = sim.NewStateStore[EFSFileSystem]()
	efsMountTargets = sim.NewStateStore[EFSMountTarget]()
	efsAccessPoints = sim.NewStateStore[EFSAccessPoint]()
	efsLifecyclePolicies = sim.NewStateStore[[]EFSLifecyclePolicy]()

	mux := srv.Mux()

	mux.HandleFunc("POST /2015-02-01/file-systems", handleEFSCreateFileSystem)
	mux.HandleFunc("GET /2015-02-01/file-systems", handleEFSDescribeFileSystems)
	mux.HandleFunc("DELETE /2015-02-01/file-systems/{id}", handleEFSDeleteFileSystem)
	mux.HandleFunc("PUT /2015-02-01/file-systems/{id}/lifecycle-configuration", handleEFSPutLifecycleConfiguration)
	mux.HandleFunc("GET /2015-02-01/file-systems/{id}/lifecycle-configuration", handleEFSDescribeLifecycleConfiguration)

	mux.HandleFunc("POST /2015-02-01/mount-targets", handleEFSCreateMountTarget)
	mux.HandleFunc("GET /2015-02-01/mount-targets", handleEFSDescribeMountTargets)
	mux.HandleFunc("GET /2015-02-01/mount-targets/{id}/security-groups", handleEFSDescribeMountTargetSecurityGroups)
	mux.HandleFunc("PUT /2015-02-01/mount-targets/{id}/security-groups", handleEFSModifyMountTargetSecurityGroups)
	mux.HandleFunc("DELETE /2015-02-01/mount-targets/{id}", handleEFSDeleteMountTarget)

	mux.HandleFunc("POST /2015-02-01/access-points", handleEFSCreateAccessPoint)
	mux.HandleFunc("GET /2015-02-01/access-points", handleEFSDescribeAccessPoints)
	mux.HandleFunc("DELETE /2015-02-01/access-points/{id}", handleEFSDeleteAccessPoint)
}

func handleEFSCreateFileSystem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CreationToken   string   `json:"CreationToken"`
		PerformanceMode string   `json:"PerformanceMode"`
		ThroughputMode  string   `json:"ThroughputMode"`
		Tags            []EFSTag `json:"Tags"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "BadRequest", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.CreationToken == "" {
		req.CreationToken = generateUUID()
	}
	if req.PerformanceMode == "" {
		req.PerformanceMode = "generalPurpose"
	}
	if req.ThroughputMode == "" {
		req.ThroughputMode = "bursting"
	}

	fsId := "fs-" + generateUUID()[:8]

	// Extract name from tags
	var name string
	for _, tag := range req.Tags {
		if tag.Key == "Name" {
			name = tag.Value
		}
	}

	fs := EFSFileSystem{
		FileSystemId:    fsId,
		FileSystemArn:   efsArn("file-system", fsId),
		CreationTime:    time.Now().Unix(),
		LifeCycleState:  "available",
		Name:            name,
		OwnerId:         "123456789012",
		PerformanceMode: req.PerformanceMode,
		ThroughputMode:  req.ThroughputMode,
		Tags:            req.Tags,
	}
	efsFileSystems.Put(fsId, fs)

	sim.WriteJSON(w, http.StatusCreated, fs)
}

func handleEFSDescribeFileSystems(w http.ResponseWriter, r *http.Request) {
	fsId := r.URL.Query().Get("FileSystemId")

	var fileSystems []EFSFileSystem
	if fsId != "" {
		fs, ok := efsFileSystems.Get(fsId)
		if ok {
			// Update mount target count
			count := 0
			for _, mt := range efsMountTargets.List() {
				if mt.FileSystemId == fsId {
					count++
				}
			}
			fs.NumberOfMountTargets = count
			fileSystems = append(fileSystems, fs)
		}
	} else {
		fileSystems = efsFileSystems.List()
	}
	if fileSystems == nil {
		fileSystems = []EFSFileSystem{}
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"FileSystems": fileSystems,
	})
}

func handleEFSDeleteFileSystem(w http.ResponseWriter, r *http.Request) {
	id := sim.PathParam(r, "id")
	if !efsFileSystems.Delete(id) {
		sim.AWSErrorf(w, "FileSystemNotFound", http.StatusNotFound,
			"File system '%s' does not exist", id)
		return
	}
	efsLifecyclePolicies.Delete(id)
	w.WriteHeader(http.StatusNoContent)
}

func handleEFSPutLifecycleConfiguration(w http.ResponseWriter, r *http.Request) {
	fsId := sim.PathParam(r, "id")
	if _, ok := efsFileSystems.Get(fsId); !ok {
		sim.AWSErrorf(w, "FileSystemNotFound", http.StatusNotFound,
			"File system '%s' does not exist", fsId)
		return
	}

	var req struct {
		LifecyclePolicies []EFSLifecyclePolicy `json:"LifecyclePolicies"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "BadRequest", "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.LifecyclePolicies == nil {
		req.LifecyclePolicies = []EFSLifecyclePolicy{}
	}
	efsLifecyclePolicies.Put(fsId, req.LifecyclePolicies)

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"LifecyclePolicies": req.LifecyclePolicies,
	})
}

func handleEFSDescribeLifecycleConfiguration(w http.ResponseWriter, r *http.Request) {
	fsId := sim.PathParam(r, "id")
	if _, ok := efsFileSystems.Get(fsId); !ok {
		sim.AWSErrorf(w, "FileSystemNotFound", http.StatusNotFound,
			"File system '%s' does not exist", fsId)
		return
	}

	policies, ok := efsLifecyclePolicies.Get(fsId)
	if !ok {
		policies = []EFSLifecyclePolicy{}
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"LifecyclePolicies": policies,
	})
}

func handleEFSCreateMountTarget(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FileSystemId   string   `json:"FileSystemId"`
		SubnetId       string   `json:"SubnetId"`
		IpAddress      string   `json:"IpAddress"`
		SecurityGroups []string `json:"SecurityGroups"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "BadRequest", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.FileSystemId == "" || req.SubnetId == "" {
		sim.AWSError(w, "BadRequest", "FileSystemId and SubnetId are required", http.StatusBadRequest)
		return
	}

	if _, ok := efsFileSystems.Get(req.FileSystemId); !ok {
		sim.AWSErrorf(w, "FileSystemNotFound", http.StatusNotFound,
			"File system '%s' does not exist", req.FileSystemId)
		return
	}

	if req.IpAddress == "" {
		req.IpAddress = fmt.Sprintf("10.0.1.%d", efsMountTargets.Len()+10)
	}

	mtId := "fsmt-" + generateUUID()[:8]
	mt := EFSMountTarget{
		MountTargetId:        mtId,
		FileSystemId:         req.FileSystemId,
		SubnetId:             req.SubnetId,
		IpAddress:            req.IpAddress,
		LifeCycleState:       "available",
		NetworkInterfaceId:   "eni-" + generateUUID()[:8],
		AvailabilityZoneId:   "use1-az1",
		AvailabilityZoneName: "us-east-1a",
		OwnerId:              "123456789012",
		SecurityGroups:       req.SecurityGroups,
	}
	efsMountTargets.Put(mtId, mt)

	sim.WriteJSON(w, http.StatusOK, mt)
}

func handleEFSDescribeMountTargets(w http.ResponseWriter, r *http.Request) {
	fsId := r.URL.Query().Get("FileSystemId")
	mtId := r.URL.Query().Get("MountTargetId")

	var targets []EFSMountTarget
	if mtId != "" {
		mt, ok := efsMountTargets.Get(mtId)
		if ok {
			targets = append(targets, mt)
		}
	} else if fsId != "" {
		targets = efsMountTargets.Filter(func(mt EFSMountTarget) bool {
			return mt.FileSystemId == fsId
		})
	} else {
		targets = efsMountTargets.List()
	}
	if targets == nil {
		targets = []EFSMountTarget{}
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"MountTargets": targets,
	})
}

func handleEFSCreateAccessPoint(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FileSystemId  string            `json:"FileSystemId"`
		PosixUser     *EFSPosixUser     `json:"PosixUser"`
		RootDirectory *EFSRootDirectory `json:"RootDirectory"`
		Tags          []EFSTag          `json:"Tags"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "BadRequest", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.FileSystemId == "" {
		sim.AWSError(w, "BadRequest", "FileSystemId is required", http.StatusBadRequest)
		return
	}

	if _, ok := efsFileSystems.Get(req.FileSystemId); !ok {
		sim.AWSErrorf(w, "FileSystemNotFound", http.StatusNotFound,
			"File system '%s' does not exist", req.FileSystemId)
		return
	}

	apId := "fsap-" + generateUUID()[:8]

	var name string
	for _, tag := range req.Tags {
		if tag.Key == "Name" {
			name = tag.Value
		}
	}

	ap := EFSAccessPoint{
		AccessPointId:  apId,
		AccessPointArn: efsArn("access-point", apId),
		FileSystemId:   req.FileSystemId,
		LifeCycleState: "available",
		Name:           name,
		OwnerId:        "123456789012",
		RootDirectory:  req.RootDirectory,
		PosixUser:      req.PosixUser,
		Tags:           req.Tags,
	}
	efsAccessPoints.Put(apId, ap)

	sim.WriteJSON(w, http.StatusOK, ap)
}

func handleEFSDescribeAccessPoints(w http.ResponseWriter, r *http.Request) {
	apId := r.URL.Query().Get("AccessPointId")
	fsId := r.URL.Query().Get("FileSystemId")

	var accessPoints []EFSAccessPoint
	if apId != "" {
		ap, ok := efsAccessPoints.Get(apId)
		if ok {
			accessPoints = append(accessPoints, ap)
		}
	} else if fsId != "" {
		accessPoints = efsAccessPoints.Filter(func(ap EFSAccessPoint) bool {
			return ap.FileSystemId == fsId
		})
	} else {
		accessPoints = efsAccessPoints.List()
	}
	if accessPoints == nil {
		accessPoints = []EFSAccessPoint{}
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"AccessPoints": accessPoints,
	})
}

func handleEFSDeleteAccessPoint(w http.ResponseWriter, r *http.Request) {
	id := sim.PathParam(r, "id")
	if id == "" {
		// Try extracting from path manually
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) > 0 {
			id = parts[len(parts)-1]
		}
	}
	if !efsAccessPoints.Delete(id) {
		sim.AWSErrorf(w, "AccessPointNotFound", http.StatusNotFound,
			"Access point '%s' does not exist", id)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleEFSDescribeMountTargetSecurityGroups(w http.ResponseWriter, r *http.Request) {
	mtId := sim.PathParam(r, "id")
	mt, ok := efsMountTargets.Get(mtId)
	if !ok {
		sim.AWSErrorf(w, "MountTargetNotFound", http.StatusNotFound,
			"Mount target '%s' does not exist", mtId)
		return
	}

	sgs := mt.SecurityGroups
	if sgs == nil {
		sgs = []string{}
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"SecurityGroups": sgs,
	})
}

func handleEFSModifyMountTargetSecurityGroups(w http.ResponseWriter, r *http.Request) {
	mtId := sim.PathParam(r, "id")
	if _, ok := efsMountTargets.Get(mtId); !ok {
		sim.AWSErrorf(w, "MountTargetNotFound", http.StatusNotFound,
			"Mount target '%s' does not exist", mtId)
		return
	}

	var req struct {
		SecurityGroups []string `json:"SecurityGroups"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "BadRequest", "Invalid request body", http.StatusBadRequest)
		return
	}

	efsMountTargets.Update(mtId, func(mt *EFSMountTarget) {
		mt.SecurityGroups = req.SecurityGroups
	})

	w.WriteHeader(http.StatusNoContent)
}

func handleEFSDeleteMountTarget(w http.ResponseWriter, r *http.Request) {
	id := sim.PathParam(r, "id")
	if !efsMountTargets.Delete(id) {
		sim.AWSErrorf(w, "MountTargetNotFound", http.StatusNotFound,
			"Mount target '%s' does not exist", id)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
