package main

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// This file implements the EC2 EBS-encryption-by-default, fast-snapshot-restore,
// snapshot-tiering, snapshot Recycle Bin, snapshot block-public-access and volume
// attribute families. EC2 speaks the ec2Query protocol: actions are registered by
// name and respond with XML whose element/list casing matches the smithy model's
// ec2QueryName / xmlName traits exactly.

// ebsEncryptionState is the account+Region-level EBS encryption-by-default
// singleton: a flag plus the default KMS key id. Real EC2 returns the AWS-managed
// key (alias/aws/ebs) when no customer key is set; ResetEbsDefaultKmsKeyId clears
// any customer key and reverts to that managed default.
type ebsEncryptionState struct {
	mu                   sync.Mutex
	enabledByDefault     bool
	customerDefaultKeyID string
}

// awsManagedEBSKeyARN is the ARN of the AWS-managed KMS key (alias/aws/ebs) used
// for EBS encryption by default when no customer-managed key is configured.
func awsManagedEBSKeyARN() string {
	return fmt.Sprintf("arn:aws:kms:%s:%s:key/aws/ebs", awsRegion(), awsAccountID())
}

func (s *ebsEncryptionState) defaultKeyID() string {
	if s.customerDefaultKeyID != "" {
		return s.customerDefaultKeyID
	}
	return awsManagedEBSKeyARN()
}

var ec2EBSEncryption = &ebsEncryptionState{}

// fastSnapshotRestore is one (snapshot, AvailabilityZone) fast-snapshot-restore
// association. Real EC2 transitions enabling -> optimizing -> enabled; the sim
// reports enabling on creation and lets DescribeFastSnapshotRestores settle it to
// enabled so a poll-until-enabled client succeeds without synthetic timers.
type fastSnapshotRestore struct {
	SnapshotID            string
	AvailabilityZone      string
	State                 string
	StateTransitionReason string
	OwnerID               string
	EnablingTime          time.Time
	EnabledTime           time.Time
	DisablingTime         time.Time
	DisabledTime          time.Time
}

// snapshotTierState tracks the storage tier (standard|archive) and, for
// temporarily restored archived snapshots, the restore expiry. It is keyed by
// snapshot id and lives alongside the main ec2Snapshots store.
type snapshotTierState struct {
	StorageTier      string
	TieringStartTime time.Time
	RestoreStartTime time.Time
	RestoreExpiry    time.Time
}

// snapshotBlockPublicAccess is the account+Region-level snapshot
// block-public-access singleton: block-all-sharing | block-new-sharing | unblocked.
type snapshotBlockPublicAccess struct {
	mu    sync.Mutex
	state string
}

var ec2SnapshotBPA = &snapshotBlockPublicAccess{state: "unblocked"}

// volumeAttributeState holds the autoEnableIO attribute per volume id. Real EC2
// defaults autoEnableIO to false on a fresh volume.
var (
	ec2FSRMu sync.Mutex
	// ec2FSR is keyed by "<snapshotId>|<az>".
	ec2FSR = map[string]*fastSnapshotRestore{}

	ec2SnapTierMu sync.Mutex
	ec2SnapTier   = map[string]*snapshotTierState{}

	ec2VolAutoEnableIOMu sync.Mutex
	ec2VolAutoEnableIO   = map[string]bool{}
)

// ec2RecycledSnapshots holds snapshots in the Recycle Bin (state "recoverable").
// Real EC2 moves a deleted snapshot here when a Recycle Bin retention rule covers
// it; RestoreSnapshotFromRecycleBin pulls it back into ec2Snapshots.
var ec2RecycledSnapshots sim.Store[EC2Snapshot]

func registerEC2EBSSnapshot(r *sim.AWSQueryRouter, srv *sim.Server) {
	ec2RecycledSnapshots = sim.MakeStore[EC2Snapshot](srv.DB(), "ec2_recycled_snapshots")

	// EBS encryption by default
	r.Register("EnableEbsEncryptionByDefault", handleEnableEbsEncryptionByDefault)
	r.Register("DisableEbsEncryptionByDefault", handleDisableEbsEncryptionByDefault)
	r.Register("GetEbsEncryptionByDefault", handleGetEbsEncryptionByDefault)
	r.Register("ModifyEbsDefaultKmsKeyId", handleModifyEbsDefaultKmsKeyId)
	r.Register("GetEbsDefaultKmsKeyId", handleGetEbsDefaultKmsKeyId)
	r.Register("ResetEbsDefaultKmsKeyId", handleResetEbsDefaultKmsKeyId)

	// Fast snapshot restores
	r.Register("EnableFastSnapshotRestores", handleEnableFastSnapshotRestores)
	r.Register("DisableFastSnapshotRestores", handleDisableFastSnapshotRestores)
	r.Register("DescribeFastSnapshotRestores", handleDescribeFastSnapshotRestores)

	// Snapshot tiering
	r.Register("ModifySnapshotTier", handleModifySnapshotTier)
	r.Register("RestoreSnapshotTier", handleRestoreSnapshotTier)

	// Snapshot Recycle Bin
	r.Register("RestoreSnapshotFromRecycleBin", handleRestoreSnapshotFromRecycleBin)

	// Snapshot block public access
	r.Register("EnableSnapshotBlockPublicAccess", handleEnableSnapshotBlockPublicAccess)
	r.Register("DisableSnapshotBlockPublicAccess", handleDisableSnapshotBlockPublicAccess)
	r.Register("GetSnapshotBlockPublicAccessState", handleGetSnapshotBlockPublicAccessState)

	// Volume attributes
	r.Register("DescribeVolumeAttribute", handleDescribeVolumeAttribute)
	r.Register("ModifyVolumeAttribute", handleModifyVolumeAttribute)
}

// ----------------------------------------------------------------------------
// EBS encryption by default
// ----------------------------------------------------------------------------

func handleEnableEbsEncryptionByDefault(w http.ResponseWriter, r *http.Request) {
	ec2EBSEncryption.mu.Lock()
	ec2EBSEncryption.enabledByDefault = true
	enabled := ec2EBSEncryption.enabledByDefault
	ec2EBSEncryption.mu.Unlock()
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<EnableEbsEncryptionByDefaultResponse %s><requestId>%s</requestId><ebsEncryptionByDefault>%t</ebsEncryptionByDefault></EnableEbsEncryptionByDefaultResponse>`,
		ec2Xmlns(), generateUUID(), enabled)
}

func handleDisableEbsEncryptionByDefault(w http.ResponseWriter, r *http.Request) {
	ec2EBSEncryption.mu.Lock()
	ec2EBSEncryption.enabledByDefault = false
	enabled := ec2EBSEncryption.enabledByDefault
	ec2EBSEncryption.mu.Unlock()
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DisableEbsEncryptionByDefaultResponse %s><requestId>%s</requestId><ebsEncryptionByDefault>%t</ebsEncryptionByDefault></DisableEbsEncryptionByDefaultResponse>`,
		ec2Xmlns(), generateUUID(), enabled)
}

func handleGetEbsEncryptionByDefault(w http.ResponseWriter, r *http.Request) {
	ec2EBSEncryption.mu.Lock()
	enabled := ec2EBSEncryption.enabledByDefault
	ec2EBSEncryption.mu.Unlock()
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<GetEbsEncryptionByDefaultResponse %s><requestId>%s</requestId><ebsEncryptionByDefault>%t</ebsEncryptionByDefault><sseType>sse-ebs</sseType></GetEbsEncryptionByDefaultResponse>`,
		ec2Xmlns(), generateUUID(), enabled)
}

func handleModifyEbsDefaultKmsKeyId(w http.ResponseWriter, r *http.Request) {
	keyID := r.FormValue("KmsKeyId")
	if keyID == "" {
		ec2ErrorXML(w, "MissingParameter", "The request must contain the parameter KmsKeyId", http.StatusBadRequest)
		return
	}
	ec2EBSEncryption.mu.Lock()
	ec2EBSEncryption.customerDefaultKeyID = keyID
	out := ec2EBSEncryption.defaultKeyID()
	ec2EBSEncryption.mu.Unlock()
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<ModifyEbsDefaultKmsKeyIdResponse %s><requestId>%s</requestId><kmsKeyId>%s</kmsKeyId></ModifyEbsDefaultKmsKeyIdResponse>`,
		ec2Xmlns(), generateUUID(), xmlEscape(out))
}

func handleGetEbsDefaultKmsKeyId(w http.ResponseWriter, r *http.Request) {
	ec2EBSEncryption.mu.Lock()
	out := ec2EBSEncryption.defaultKeyID()
	ec2EBSEncryption.mu.Unlock()
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<GetEbsDefaultKmsKeyIdResponse %s><requestId>%s</requestId><kmsKeyId>%s</kmsKeyId></GetEbsDefaultKmsKeyIdResponse>`,
		ec2Xmlns(), generateUUID(), xmlEscape(out))
}

func handleResetEbsDefaultKmsKeyId(w http.ResponseWriter, r *http.Request) {
	ec2EBSEncryption.mu.Lock()
	ec2EBSEncryption.customerDefaultKeyID = ""
	out := ec2EBSEncryption.defaultKeyID()
	ec2EBSEncryption.mu.Unlock()
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<ResetEbsDefaultKmsKeyIdResponse %s><requestId>%s</requestId><kmsKeyId>%s</kmsKeyId></ResetEbsDefaultKmsKeyIdResponse>`,
		ec2Xmlns(), generateUUID(), xmlEscape(out))
}

// ----------------------------------------------------------------------------
// Fast snapshot restores
// ----------------------------------------------------------------------------

func fsrKey(snapID, az string) string { return snapID + "|" + az }

// ec2SettleFSR advances an enabling/disabling association to its terminal state,
// matching real EC2's enabling -> optimizing -> enabled progression (the sim
// collapses optimizing). No synthetic timer: the transition completes on the next
// read, like the snapshot pending -> completed settle.
func ec2SettleFSR(f *fastSnapshotRestore) {
	switch f.State {
	case "enabling", "optimizing":
		f.State = "enabled"
		f.StateTransitionReason = "Client.UserInitiated - Lifecycle state transition"
		if f.EnabledTime.IsZero() {
			f.EnabledTime = time.Now().UTC()
		}
	case "disabling":
		f.State = "disabled"
		f.StateTransitionReason = "Client.UserInitiated - Lifecycle state transition"
		if f.DisabledTime.IsZero() {
			f.DisabledTime = time.Now().UTC()
		}
	}
}

func handleEnableFastSnapshotRestores(w http.ResponseWriter, r *http.Request) {
	azs := ec2ParamList(r, "AvailabilityZone")
	snaps := ec2ParamList(r, "SourceSnapshotId")
	if len(azs) == 0 || len(snaps) == 0 {
		ec2ErrorXML(w, "MissingParameter", "AvailabilityZone and SourceSnapshotId are required", http.StatusBadRequest)
		return
	}
	type succ struct {
		snapID, az string
		f          *fastSnapshotRestore
	}
	type errItem struct {
		snapID, az, msg string
	}
	var successful []succ
	var unsuccessful []errItem
	now := time.Now().UTC()
	for _, snapID := range snaps {
		_, exists := ec2Snapshots.Get(snapID)
		for _, az := range azs {
			if !exists {
				unsuccessful = append(unsuccessful, errItem{snapID, az, "The snapshot does not exist."})
				continue
			}
			ec2FSRMu.Lock()
			f := &fastSnapshotRestore{
				SnapshotID:            snapID,
				AvailabilityZone:      az,
				State:                 "enabling",
				StateTransitionReason: "Client.UserInitiated",
				OwnerID:               awsAccountID(),
				EnablingTime:          now,
			}
			ec2FSR[fsrKey(snapID, az)] = f
			ec2FSRMu.Unlock()
			successful = append(successful, succ{snapID, az, f})
		}
	}
	w.Header().Set("Content-Type", "text/xml")
	var b strings.Builder
	fmt.Fprintf(&b, `<EnableFastSnapshotRestoresResponse %s><requestId>%s</requestId><successful>`, ec2Xmlns(), generateUUID())
	for _, s := range successful {
		b.WriteString(fsrItemXML(*s.f))
	}
	b.WriteString(`</successful><unsuccessful>`)
	for _, e := range unsuccessful {
		fmt.Fprintf(&b, `<item><snapshotId>%s</snapshotId><fastSnapshotRestoreStateErrorSet><item><availabilityZone>%s</availabilityZone><error><code>InvalidSnapshot.NotFound</code><message>%s</message></error></item></fastSnapshotRestoreStateErrorSet></item>`,
			e.snapID, e.az, xmlEscape(e.msg))
	}
	b.WriteString(`</unsuccessful></EnableFastSnapshotRestoresResponse>`)
	_, _ = w.Write([]byte(b.String()))
}

func handleDisableFastSnapshotRestores(w http.ResponseWriter, r *http.Request) {
	azs := ec2ParamList(r, "AvailabilityZone")
	snaps := ec2ParamList(r, "SourceSnapshotId")
	if len(azs) == 0 || len(snaps) == 0 {
		ec2ErrorXML(w, "MissingParameter", "AvailabilityZone and SourceSnapshotId are required", http.StatusBadRequest)
		return
	}
	type errItem struct{ snapID, az, msg string }
	var successful []fastSnapshotRestore
	var unsuccessful []errItem
	now := time.Now().UTC()
	for _, snapID := range snaps {
		for _, az := range azs {
			ec2FSRMu.Lock()
			f, ok := ec2FSR[fsrKey(snapID, az)]
			if !ok {
				ec2FSRMu.Unlock()
				unsuccessful = append(unsuccessful, errItem{snapID, az, "Fast snapshot restore is not enabled for the specified Availability Zone."})
				continue
			}
			f.State = "disabling"
			f.StateTransitionReason = "Client.UserInitiated"
			f.DisablingTime = now
			snapshot := *f
			ec2FSRMu.Unlock()
			successful = append(successful, snapshot)
		}
	}
	w.Header().Set("Content-Type", "text/xml")
	var b strings.Builder
	fmt.Fprintf(&b, `<DisableFastSnapshotRestoresResponse %s><requestId>%s</requestId><successful>`, ec2Xmlns(), generateUUID())
	for _, s := range successful {
		b.WriteString(fsrItemXML(s))
	}
	b.WriteString(`</successful><unsuccessful>`)
	for _, e := range unsuccessful {
		fmt.Fprintf(&b, `<item><snapshotId>%s</snapshotId><fastSnapshotRestoreStateErrorSet><item><availabilityZone>%s</availabilityZone><error><code>InvalidParameterValue</code><message>%s</message></error></item></fastSnapshotRestoreStateErrorSet></item>`,
			e.snapID, e.az, xmlEscape(e.msg))
	}
	b.WriteString(`</unsuccessful></DisableFastSnapshotRestoresResponse>`)
	_, _ = w.Write([]byte(b.String()))
}

func handleDescribeFastSnapshotRestores(w http.ResponseWriter, r *http.Request) {
	filters := ec2Filters(r)
	ec2FSRMu.Lock()
	var items []fastSnapshotRestore
	for _, f := range ec2FSR {
		ec2SettleFSR(f)
		items = append(items, *f)
	}
	ec2FSRMu.Unlock()
	var matched []fastSnapshotRestore
	for _, f := range items {
		if !fsrMatchesFilters(f, filters) {
			continue
		}
		matched = append(matched, f)
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].SnapshotID != matched[j].SnapshotID {
			return matched[i].SnapshotID < matched[j].SnapshotID
		}
		return matched[i].AvailabilityZone < matched[j].AvailabilityZone
	})
	w.Header().Set("Content-Type", "text/xml")
	var b strings.Builder
	fmt.Fprintf(&b, `<DescribeFastSnapshotRestoresResponse %s><requestId>%s</requestId><fastSnapshotRestoreSet>`, ec2Xmlns(), generateUUID())
	for _, f := range matched {
		b.WriteString(fsrItemXML(f))
	}
	b.WriteString(`</fastSnapshotRestoreSet></DescribeFastSnapshotRestoresResponse>`)
	_, _ = w.Write([]byte(b.String()))
}

func fsrMatchesFilters(f fastSnapshotRestore, filters map[string][]string) bool {
	for name, vals := range filters {
		switch name {
		case "snapshot-id":
			if !ec2StrInValues(f.SnapshotID, vals) {
				return false
			}
		case "availability-zone":
			if !ec2StrInValues(f.AvailabilityZone, vals) {
				return false
			}
		case "state":
			if !ec2StrInValues(f.State, vals) {
				return false
			}
		case "owner-id":
			if !ec2StrInValues(f.OwnerID, vals) {
				return false
			}
		}
	}
	return true
}

func fsrTimeXML(tag string, t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return fmt.Sprintf("<%s>%s</%s>", tag, t.Format(time.RFC3339), tag)
}

func fsrItemXML(f fastSnapshotRestore) string {
	var b strings.Builder
	b.WriteString("<item>")
	fmt.Fprintf(&b, "<snapshotId>%s</snapshotId>", f.SnapshotID)
	fmt.Fprintf(&b, "<availabilityZone>%s</availabilityZone>", f.AvailabilityZone)
	fmt.Fprintf(&b, "<state>%s</state>", f.State)
	fmt.Fprintf(&b, "<stateTransitionReason>%s</stateTransitionReason>", xmlEscape(f.StateTransitionReason))
	fmt.Fprintf(&b, "<ownerId>%s</ownerId>", f.OwnerID)
	b.WriteString(fsrTimeXML("enablingTime", f.EnablingTime))
	b.WriteString(fsrTimeXML("enabledTime", f.EnabledTime))
	b.WriteString(fsrTimeXML("disablingTime", f.DisablingTime))
	b.WriteString(fsrTimeXML("disabledTime", f.DisabledTime))
	b.WriteString("</item>")
	return b.String()
}

// ----------------------------------------------------------------------------
// Snapshot tiering
// ----------------------------------------------------------------------------

func handleModifySnapshotTier(w http.ResponseWriter, r *http.Request) {
	snapID := r.FormValue("SnapshotId")
	if snapID == "" {
		ec2ErrorXML(w, "MissingParameter", "The request must contain the parameter SnapshotId", http.StatusBadRequest)
		return
	}
	if _, ok := ec2Snapshots.Get(snapID); !ok {
		ec2ErrorXML(w, "InvalidSnapshot.NotFound", fmt.Sprintf("The snapshot %q does not exist", snapID), http.StatusBadRequest)
		return
	}
	tier := r.FormValue("StorageTier")
	if tier != "archive" {
		ec2ErrorXML(w, "InvalidParameterValue", fmt.Sprintf("Value (%s) for parameter StorageTier is invalid. The only supported value is archive.", tier), http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	ec2SnapTierMu.Lock()
	ec2SnapTier[snapID] = &snapshotTierState{StorageTier: "archive", TieringStartTime: now}
	ec2SnapTierMu.Unlock()
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<ModifySnapshotTierResponse %s><requestId>%s</requestId><snapshotId>%s</snapshotId><tieringStartTime>%s</tieringStartTime></ModifySnapshotTierResponse>`,
		ec2Xmlns(), generateUUID(), snapID, now.Format(time.RFC3339))
}

func handleRestoreSnapshotTier(w http.ResponseWriter, r *http.Request) {
	snapID := r.FormValue("SnapshotId")
	if snapID == "" {
		ec2ErrorXML(w, "MissingParameter", "The request must contain the parameter SnapshotId", http.StatusBadRequest)
		return
	}
	if _, ok := ec2Snapshots.Get(snapID); !ok {
		ec2ErrorXML(w, "InvalidSnapshot.NotFound", fmt.Sprintf("The snapshot %q does not exist", snapID), http.StatusBadRequest)
		return
	}
	permanent := ec2BoolStr(r.FormValue("PermanentRestore"))
	restoreDays := ec2AtoiOr(r.FormValue("TemporaryRestoreDays"), 0)
	if !permanent && restoreDays <= 0 {
		ec2ErrorXML(w, "InvalidParameterValue", "Either PermanentRestore or TemporaryRestoreDays must be specified.", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	ec2SnapTierMu.Lock()
	st := ec2SnapTier[snapID]
	if st == nil {
		st = &snapshotTierState{}
		ec2SnapTier[snapID] = st
	}
	st.StorageTier = "standard"
	st.RestoreStartTime = now
	if permanent {
		st.RestoreExpiry = time.Time{}
	} else {
		st.RestoreExpiry = now.AddDate(0, 0, restoreDays)
	}
	ec2SnapTierMu.Unlock()
	w.Header().Set("Content-Type", "text/xml")
	var b strings.Builder
	fmt.Fprintf(&b, `<RestoreSnapshotTierResponse %s><requestId>%s</requestId><snapshotId>%s</snapshotId><restoreStartTime>%s</restoreStartTime>`,
		ec2Xmlns(), generateUUID(), snapID, now.Format(time.RFC3339))
	if !permanent {
		fmt.Fprintf(&b, "<restoreDuration>%d</restoreDuration>", restoreDays)
	}
	fmt.Fprintf(&b, "<isPermanentRestore>%t</isPermanentRestore></RestoreSnapshotTierResponse>", permanent)
	_, _ = w.Write([]byte(b.String()))
}

// ----------------------------------------------------------------------------
// Snapshot Recycle Bin
// ----------------------------------------------------------------------------

func handleRestoreSnapshotFromRecycleBin(w http.ResponseWriter, r *http.Request) {
	snapID := r.FormValue("SnapshotId")
	if snapID == "" {
		ec2ErrorXML(w, "MissingParameter", "The request must contain the parameter SnapshotId", http.StatusBadRequest)
		return
	}
	// A snapshot is restorable only if it is in the Recycle Bin (state
	// "recoverable"). Restore it to the live store as "completed" and emit the
	// restored snapshot's metadata, matching real EC2.
	if snap, ok := ec2RecycledSnapshots.Get(snapID); ok {
		snap.State = "completed"
		snap.Progress = "100%"
		ec2RecycledSnapshots.Delete(snapID)
		ec2Snapshots.Put(snapID, snap)
		writeRestoreSnapshotFromRecycleBinXML(w, snap)
		return
	}
	if snap, ok := ec2Snapshots.Get(snapID); ok && snap.State == "recoverable" {
		snap.State = "completed"
		snap.Progress = "100%"
		ec2Snapshots.Put(snapID, snap)
		writeRestoreSnapshotFromRecycleBinXML(w, snap)
		return
	}
	// Live or non-existent snapshot: not in the Recycle Bin. Real EC2 returns
	// InvalidSnapshot.NotFound for a snapshot that isn't recoverable.
	ec2ErrorXML(w, "InvalidSnapshot.NotFound", fmt.Sprintf("The snapshot %q is not in the Recycle Bin", snapID), http.StatusBadRequest)
}

func writeRestoreSnapshotFromRecycleBinXML(w http.ResponseWriter, snap EC2Snapshot) {
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<RestoreSnapshotFromRecycleBinResponse %s><requestId>%s</requestId><snapshotId>%s</snapshotId><description>%s</description><encrypted>%t</encrypted><ownerId>%s</ownerId><progress>%s</progress><startTime>%s</startTime><status>%s</status><volumeId>%s</volumeId><volumeSize>%d</volumeSize></RestoreSnapshotFromRecycleBinResponse>`,
		ec2Xmlns(), generateUUID(), snap.SnapshotId, xmlEscape(snap.Description), snap.Encrypted,
		snap.OwnerId, snap.Progress, snap.StartTime, snap.State, snap.VolumeId, snap.VolumeSize)
}

// ----------------------------------------------------------------------------
// Snapshot block public access
// ----------------------------------------------------------------------------

func handleEnableSnapshotBlockPublicAccess(w http.ResponseWriter, r *http.Request) {
	state := r.FormValue("State")
	if state != "block-all-sharing" && state != "block-new-sharing" {
		ec2ErrorXML(w, "InvalidParameterValue", fmt.Sprintf("Value (%s) for parameter State is invalid. Valid values are block-all-sharing and block-new-sharing.", state), http.StatusBadRequest)
		return
	}
	ec2SnapshotBPA.mu.Lock()
	ec2SnapshotBPA.state = state
	ec2SnapshotBPA.mu.Unlock()
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<EnableSnapshotBlockPublicAccessResponse %s><requestId>%s</requestId><state>%s</state></EnableSnapshotBlockPublicAccessResponse>`,
		ec2Xmlns(), generateUUID(), state)
}

func handleDisableSnapshotBlockPublicAccess(w http.ResponseWriter, r *http.Request) {
	ec2SnapshotBPA.mu.Lock()
	ec2SnapshotBPA.state = "unblocked"
	ec2SnapshotBPA.mu.Unlock()
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DisableSnapshotBlockPublicAccessResponse %s><requestId>%s</requestId><state>unblocked</state></DisableSnapshotBlockPublicAccessResponse>`,
		ec2Xmlns(), generateUUID())
}

func handleGetSnapshotBlockPublicAccessState(w http.ResponseWriter, r *http.Request) {
	ec2SnapshotBPA.mu.Lock()
	state := ec2SnapshotBPA.state
	ec2SnapshotBPA.mu.Unlock()
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<GetSnapshotBlockPublicAccessStateResponse %s><requestId>%s</requestId><state>%s</state><managedBy>account</managedBy></GetSnapshotBlockPublicAccessStateResponse>`,
		ec2Xmlns(), generateUUID(), state)
}

// ----------------------------------------------------------------------------
// Volume attributes
// ----------------------------------------------------------------------------

func handleDescribeVolumeAttribute(w http.ResponseWriter, r *http.Request) {
	volID := r.FormValue("VolumeId")
	attr := r.FormValue("Attribute")
	if volID == "" || attr == "" {
		ec2ErrorXML(w, "MissingParameter", "VolumeId and Attribute are required", http.StatusBadRequest)
		return
	}
	if _, ok := ec2Volumes.Get(volID); !ok {
		ec2ErrorXML(w, "InvalidVolume.NotFound", fmt.Sprintf("The volume %q does not exist", volID), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/xml")
	var b strings.Builder
	fmt.Fprintf(&b, `<DescribeVolumeAttributeResponse %s><requestId>%s</requestId><volumeId>%s</volumeId>`,
		ec2Xmlns(), generateUUID(), volID)
	switch attr {
	case "autoEnableIO":
		ec2VolAutoEnableIOMu.Lock()
		val := ec2VolAutoEnableIO[volID]
		ec2VolAutoEnableIOMu.Unlock()
		fmt.Fprintf(&b, "<autoEnableIO><value>%t</value></autoEnableIO>", val)
	case "productCodes":
		b.WriteString("<productCodes/>")
	default:
		ec2ErrorXML(w, "InvalidParameterValue", fmt.Sprintf("Value (%s) for parameter Attribute is invalid.", attr), http.StatusBadRequest)
		return
	}
	b.WriteString("</DescribeVolumeAttributeResponse>")
	_, _ = w.Write([]byte(b.String()))
}

func handleModifyVolumeAttribute(w http.ResponseWriter, r *http.Request) {
	volID := r.FormValue("VolumeId")
	if volID == "" {
		ec2ErrorXML(w, "MissingParameter", "The request must contain the parameter VolumeId", http.StatusBadRequest)
		return
	}
	if _, ok := ec2Volumes.Get(volID); !ok {
		ec2ErrorXML(w, "InvalidVolume.NotFound", fmt.Sprintf("The volume %q does not exist", volID), http.StatusBadRequest)
		return
	}
	if v := r.FormValue("AutoEnableIO.Value"); v != "" {
		ec2VolAutoEnableIOMu.Lock()
		ec2VolAutoEnableIO[volID] = ec2BoolStr(v)
		ec2VolAutoEnableIOMu.Unlock()
	}
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<ModifyVolumeAttributeResponse %s><requestId>%s</requestId><return>true</return></ModifyVolumeAttributeResponse>`,
		ec2Xmlns(), generateUUID())
}
