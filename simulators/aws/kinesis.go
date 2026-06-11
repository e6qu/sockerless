package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

type KinesisStream struct {
	StreamName           string            `json:"StreamName"`
	StreamARN            string            `json:"StreamARN"`
	StreamStatus         string            `json:"StreamStatus"`
	StreamModeDetails    map[string]string `json:"StreamModeDetails,omitempty"`
	Shards               []KinesisShard    `json:"Shards"`
	RetentionPeriodHours int64             `json:"RetentionPeriodHours"`
	EnhancedMonitoring   []map[string]any  `json:"EnhancedMonitoring"`
	EncryptionType       string            `json:"EncryptionType,omitempty"`
	KeyId                string            `json:"KeyId,omitempty"`
	CreationTimestamp    float64           `json:"-"`
	Tags                 map[string]string `json:"-"`
	OpenShardCount       int64             `json:"-"`
}

type KinesisShard struct {
	ShardId             string            `json:"ShardId"`
	HashKeyRange        map[string]string `json:"HashKeyRange"`
	SequenceNumberRange map[string]string `json:"SequenceNumberRange"`
	ParentShardId       string            `json:"ParentShardId,omitempty"`
}

type kinesisRecord struct {
	SequenceNumber              string
	ApproximateArrivalTimestamp float64
	Data                        []byte
	PartitionKey                string
	ExplicitHashKey             string
}

type kinesisIterator struct {
	StreamName string
	ShardID    string
	Index      int
}

var (
	kinesisStreams   sim.Store[KinesisStream]
	kinesisRecords   sim.Store[[]kinesisRecord]
	kinesisIterators sim.Store[kinesisIterator]
	kinesisMu        sync.Mutex
)

func registerKinesis(r *sim.AWSRouter, srv *sim.Server) {
	kinesisStreams = sim.MakeStore[KinesisStream](srv.DB(), "kinesis_streams")
	kinesisRecords = sim.MakeStore[[]kinesisRecord](srv.DB(), "kinesis_records")
	kinesisIterators = sim.MakeStore[kinesisIterator](srv.DB(), "kinesis_iterators")

	r.Register("Kinesis_20131202.CreateStream", handleKinesisCreateStream)
	r.Register("Kinesis_20131202.DeleteStream", handleKinesisDeleteStream)
	r.Register("Kinesis_20131202.DescribeStream", handleKinesisDescribeStream)
	r.Register("Kinesis_20131202.DescribeStreamSummary", handleKinesisDescribeStreamSummary)
	r.Register("Kinesis_20131202.ListStreams", handleKinesisListStreams)
	r.Register("Kinesis_20131202.ListShards", handleKinesisListShards)
	r.Register("Kinesis_20131202.PutRecord", handleKinesisPutRecord)
	r.Register("Kinesis_20131202.PutRecords", handleKinesisPutRecords)
	r.Register("Kinesis_20131202.GetShardIterator", handleKinesisGetShardIterator)
	r.Register("Kinesis_20131202.GetRecords", handleKinesisGetRecords)
	r.Register("Kinesis_20131202.AddTagsToStream", handleKinesisAddTagsToStream)
	r.Register("Kinesis_20131202.RemoveTagsFromStream", handleKinesisRemoveTagsFromStream)
	r.Register("Kinesis_20131202.ListTagsForStream", handleKinesisListTagsForStream)
	r.Register("Kinesis_20131202.IncreaseStreamRetentionPeriod", handleKinesisIncreaseStreamRetentionPeriod)
	r.Register("Kinesis_20131202.DecreaseStreamRetentionPeriod", handleKinesisDecreaseStreamRetentionPeriod)
	r.Register("Kinesis_20131202.EnableEnhancedMonitoring", handleKinesisEnableEnhancedMonitoring)
	r.Register("Kinesis_20131202.DisableEnhancedMonitoring", handleKinesisDisableEnhancedMonitoring)
	r.Register("Kinesis_20131202.StartStreamEncryption", handleKinesisStartStreamEncryption)
	r.Register("Kinesis_20131202.StopStreamEncryption", handleKinesisStopStreamEncryption)
	r.Register("Kinesis_20131202.UpdateShardCount", handleKinesisUpdateShardCount)
	r.Register("Kinesis_20131202.DescribeLimits", handleKinesisDescribeLimits)
}

func writeKinesisJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func kinesisStreamARN(name string) string {
	return fmt.Sprintf("arn:aws:kinesis:%s:%s:stream/%s", awsRegion(), awsAccountID(), name)
}

func kinesisShardRecordKey(streamName, shardID string) string {
	return streamName + "/" + shardID
}

func kinesisMakeShards(count int64) []KinesisShard {
	if count < 1 {
		count = 1
	}
	maxHash := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))
	denom := big.NewInt(count)
	shards := make([]KinesisShard, 0, count)
	for i := int64(0); i < count; i++ {
		start := new(big.Int).Div(new(big.Int).Mul(maxHash, big.NewInt(i)), denom)
		end := new(big.Int).Div(new(big.Int).Mul(maxHash, big.NewInt(i+1)), denom)
		if i > 0 {
			start.Add(start, big.NewInt(1))
		}
		shards = append(shards, KinesisShard{
			ShardId: fmt.Sprintf("shardId-%012d", i),
			HashKeyRange: map[string]string{
				"StartingHashKey": start.String(),
				"EndingHashKey":   end.String(),
			},
			SequenceNumberRange: map[string]string{
				"StartingSequenceNumber": "1",
			},
		})
	}
	return shards
}

func kinesisStreamDescription(s KinesisStream) map[string]any {
	out := map[string]any{
		"StreamName":              s.StreamName,
		"StreamARN":               s.StreamARN,
		"StreamStatus":            s.StreamStatus,
		"StreamModeDetails":       s.StreamModeDetails,
		"Shards":                  s.Shards,
		"HasMoreShards":           false,
		"RetentionPeriodHours":    s.RetentionPeriodHours,
		"StreamCreationTimestamp": s.CreationTimestamp,
		"EnhancedMonitoring":      s.EnhancedMonitoring,
	}
	kinesisSetEncryption(out, s)
	return out
}

// kinesisStreamDescriptionSummary backs DescribeStreamSummary, whose
// StreamDescriptionSummary shape carries the shard/retention/monitoring
// and encryption members.
func kinesisStreamDescriptionSummary(s KinesisStream) map[string]any {
	open := s.OpenShardCount
	if open == 0 {
		open = int64(len(s.Shards))
	}
	out := map[string]any{
		"StreamName":              s.StreamName,
		"StreamARN":               s.StreamARN,
		"StreamStatus":            s.StreamStatus,
		"RetentionPeriodHours":    s.RetentionPeriodHours,
		"StreamCreationTimestamp": s.CreationTimestamp,
		"EnhancedMonitoring":      s.EnhancedMonitoring,
		"OpenShardCount":          open,
		"StreamModeDetails":       s.StreamModeDetails,
	}
	kinesisSetEncryption(out, s)
	return out
}

// kinesisListStreamSummary backs ListStreams' StreamSummaries[], whose
// StreamSummary shape is the identity slice only — retention, shard
// counts, monitoring, and encryption are describe-only members.
func kinesisListStreamSummary(s KinesisStream) map[string]any {
	return map[string]any{
		"StreamName":              s.StreamName,
		"StreamARN":               s.StreamARN,
		"StreamStatus":            s.StreamStatus,
		"StreamModeDetails":       s.StreamModeDetails,
		"StreamCreationTimestamp": s.CreationTimestamp,
	}
}

// kinesisSetEncryption mirrors real Kinesis: an unencrypted stream reports
// EncryptionType=NONE and omits KeyId; a KMS-encrypted one reports its type
// and key id.
func kinesisSetEncryption(out map[string]any, s KinesisStream) {
	if s.EncryptionType == "" {
		out["EncryptionType"] = "NONE"
		return
	}
	out["EncryptionType"] = s.EncryptionType
	if s.KeyId != "" {
		out["KeyId"] = s.KeyId
	}
}

func kinesisStreamByNameOrARN(streamName, streamARN string) (KinesisStream, bool) {
	if streamName != "" {
		return kinesisStreams.Get(streamName)
	}
	if streamARN != "" {
		const sep = ":stream/"
		if idx := strings.Index(streamARN, sep); idx >= 0 {
			return kinesisStreams.Get(streamARN[idx+len(sep):])
		}
	}
	return KinesisStream{}, false
}

func handleKinesisCreateStream(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StreamName        string            `json:"StreamName"`
		ShardCount        int64             `json:"ShardCount"`
		StreamModeDetails map[string]string `json:"StreamModeDetails"`
		Tags              map[string]string `json:"Tags"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidArgumentException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.StreamName == "" {
		sim.AWSError(w, "InvalidArgumentException", "StreamName is required", http.StatusBadRequest)
		return
	}
	if _, ok := kinesisStreams.Get(req.StreamName); ok {
		sim.AWSError(w, "ResourceInUseException", "Stream already exists", http.StatusBadRequest)
		return
	}
	mode := req.StreamModeDetails
	if mode == nil {
		mode = map[string]string{"StreamMode": "PROVISIONED"}
	}
	shardCount := req.ShardCount
	if strings.EqualFold(mode["StreamMode"], "ON_DEMAND") && shardCount == 0 {
		shardCount = 4
	}
	if shardCount == 0 {
		shardCount = 1
	}
	stream := KinesisStream{
		StreamName:           req.StreamName,
		StreamARN:            kinesisStreamARN(req.StreamName),
		StreamStatus:         "ACTIVE",
		StreamModeDetails:    mode,
		Shards:               kinesisMakeShards(shardCount),
		RetentionPeriodHours: 24,
		EnhancedMonitoring:   []map[string]any{{"ShardLevelMetrics": []string{}}},
		CreationTimestamp:    float64(time.Now().Unix()),
		Tags:                 map[string]string{},
		OpenShardCount:       shardCount,
	}
	for k, v := range req.Tags {
		stream.Tags[k] = v
	}
	kinesisStreams.Put(req.StreamName, stream)
	writeKinesisJSON(w, http.StatusOK, map[string]any{})
}

func handleKinesisDeleteStream(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StreamName string `json:"StreamName"`
		StreamARN  string `json:"StreamARN"`
	}
	_ = sim.ReadJSON(r, &req)
	stream, ok := kinesisStreamByNameOrARN(req.StreamName, req.StreamARN)
	if !ok {
		writeKinesisJSON(w, http.StatusOK, map[string]any{})
		return
	}
	kinesisStreams.Delete(stream.StreamName)
	for _, shard := range stream.Shards {
		kinesisRecords.Delete(kinesisShardRecordKey(stream.StreamName, shard.ShardId))
	}
	writeKinesisJSON(w, http.StatusOK, map[string]any{})
}

func handleKinesisDescribeStream(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StreamName string `json:"StreamName"`
		StreamARN  string `json:"StreamARN"`
	}
	_ = sim.ReadJSON(r, &req)
	stream, ok := kinesisStreamByNameOrARN(req.StreamName, req.StreamARN)
	if !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Stream not found", http.StatusBadRequest)
		return
	}
	writeKinesisJSON(w, http.StatusOK, map[string]any{"StreamDescription": kinesisStreamDescription(stream)})
}

func handleKinesisDescribeStreamSummary(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StreamName string `json:"StreamName"`
		StreamARN  string `json:"StreamARN"`
	}
	_ = sim.ReadJSON(r, &req)
	stream, ok := kinesisStreamByNameOrARN(req.StreamName, req.StreamARN)
	if !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Stream not found", http.StatusBadRequest)
		return
	}
	writeKinesisJSON(w, http.StatusOK, map[string]any{"StreamDescriptionSummary": kinesisStreamDescriptionSummary(stream)})
}

func handleKinesisListStreams(w http.ResponseWriter, r *http.Request) {
	var names []string
	for _, stream := range kinesisStreams.List() {
		names = append(names, stream.StreamName)
	}
	sort.Strings(names)
	writeKinesisJSON(w, http.StatusOK, map[string]any{
		"StreamNames":     names,
		"HasMoreStreams":  false,
		"StreamSummaries": kinesisStreamSummaries(names),
	})
}

func kinesisStreamSummaries(names []string) []map[string]any {
	out := make([]map[string]any, 0, len(names))
	for _, name := range names {
		if stream, ok := kinesisStreams.Get(name); ok {
			out = append(out, kinesisListStreamSummary(stream))
		}
	}
	return out
}

func handleKinesisListShards(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StreamName string `json:"StreamName"`
		StreamARN  string `json:"StreamARN"`
	}
	_ = sim.ReadJSON(r, &req)
	stream, ok := kinesisStreamByNameOrARN(req.StreamName, req.StreamARN)
	if !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Stream not found", http.StatusBadRequest)
		return
	}
	writeKinesisJSON(w, http.StatusOK, map[string]any{"Shards": stream.Shards})
}

func handleKinesisPutRecord(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StreamName      string `json:"StreamName"`
		StreamARN       string `json:"StreamARN"`
		Data            []byte `json:"Data"`
		PartitionKey    string `json:"PartitionKey"`
		ExplicitHashKey string `json:"ExplicitHashKey"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidArgumentException", "Invalid request body", http.StatusBadRequest)
		return
	}
	shardID, seq, err := kinesisAppendRecord(req.StreamName, req.StreamARN, req.Data, req.PartitionKey, req.ExplicitHashKey)
	if err != nil {
		sim.AWSError(w, "ResourceNotFoundException", err.Error(), http.StatusBadRequest)
		return
	}
	writeKinesisJSON(w, http.StatusOK, map[string]any{
		"ShardId":        shardID,
		"SequenceNumber": seq,
	})
}

func handleKinesisPutRecords(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StreamName string `json:"StreamName"`
		StreamARN  string `json:"StreamARN"`
		Records    []struct {
			Data            []byte `json:"Data"`
			PartitionKey    string `json:"PartitionKey"`
			ExplicitHashKey string `json:"ExplicitHashKey"`
		} `json:"Records"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidArgumentException", "Invalid request body", http.StatusBadRequest)
		return
	}
	out := make([]map[string]any, 0, len(req.Records))
	for _, rec := range req.Records {
		shardID, seq, err := kinesisAppendRecord(req.StreamName, req.StreamARN, rec.Data, rec.PartitionKey, rec.ExplicitHashKey)
		if err != nil {
			out = append(out, map[string]any{"ErrorCode": "ResourceNotFoundException", "ErrorMessage": err.Error()})
			continue
		}
		out = append(out, map[string]any{"ShardId": shardID, "SequenceNumber": seq})
	}
	writeKinesisJSON(w, http.StatusOK, map[string]any{
		"FailedRecordCount": 0,
		"Records":           out,
	})
}

func kinesisAppendRecord(streamName, streamARN string, data []byte, partitionKey, explicitHashKey string) (string, string, error) {
	kinesisMu.Lock()
	defer kinesisMu.Unlock()
	stream, ok := kinesisStreamByNameOrARN(streamName, streamARN)
	if !ok {
		return "", "", fmt.Errorf("stream not found")
	}
	shard := kinesisSelectShard(stream, partitionKey, explicitHashKey)
	key := kinesisShardRecordKey(stream.StreamName, shard.ShardId)
	records, _ := kinesisRecords.Get(key)
	seq := strconv.FormatInt(int64(len(records)+1), 10)
	records = append(records, kinesisRecord{
		SequenceNumber:              seq,
		ApproximateArrivalTimestamp: float64(time.Now().Unix()),
		Data:                        data,
		PartitionKey:                partitionKey,
		ExplicitHashKey:             explicitHashKey,
	})
	kinesisRecords.Put(key, records)
	return shard.ShardId, seq, nil
}

func kinesisSelectShard(stream KinesisStream, partitionKey, explicitHashKey string) KinesisShard {
	hash := new(big.Int)
	if explicitHashKey != "" {
		hash.SetString(explicitHashKey, 10)
	} else {
		sum := md5.Sum([]byte(partitionKey))
		hash.SetString(hex.EncodeToString(sum[:]), 16)
	}
	for _, shard := range stream.Shards {
		start := new(big.Int)
		end := new(big.Int)
		start.SetString(shard.HashKeyRange["StartingHashKey"], 10)
		end.SetString(shard.HashKeyRange["EndingHashKey"], 10)
		if hash.Cmp(start) >= 0 && hash.Cmp(end) <= 0 {
			return shard
		}
	}
	return stream.Shards[0]
}

func handleKinesisGetShardIterator(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StreamName             string `json:"StreamName"`
		StreamARN              string `json:"StreamARN"`
		ShardId                string `json:"ShardId"`
		ShardIteratorType      string `json:"ShardIteratorType"`
		StartingSequenceNumber string `json:"StartingSequenceNumber"`
	}
	_ = sim.ReadJSON(r, &req)
	stream, ok := kinesisStreamByNameOrARN(req.StreamName, req.StreamARN)
	if !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Stream not found", http.StatusBadRequest)
		return
	}
	if !kinesisHasShard(stream, req.ShardId) {
		sim.AWSError(w, "ResourceNotFoundException", "Shard not found", http.StatusBadRequest)
		return
	}
	records, _ := kinesisRecords.Get(kinesisShardRecordKey(stream.StreamName, req.ShardId))
	index := 0
	switch req.ShardIteratorType {
	case "LATEST":
		index = len(records)
	case "AT_SEQUENCE_NUMBER", "AFTER_SEQUENCE_NUMBER":
		if seq, err := strconv.Atoi(req.StartingSequenceNumber); err == nil && seq > 0 {
			index = seq - 1
			if req.ShardIteratorType == "AFTER_SEQUENCE_NUMBER" {
				index = seq
			}
		}
	case "", "TRIM_HORIZON":
		index = 0
	default:
		index = 0
	}
	token := generateUUID()
	kinesisIterators.Put(token, kinesisIterator{StreamName: stream.StreamName, ShardID: req.ShardId, Index: index})
	writeKinesisJSON(w, http.StatusOK, map[string]any{"ShardIterator": token})
}

func kinesisHasShard(stream KinesisStream, shardID string) bool {
	for _, shard := range stream.Shards {
		if shard.ShardId == shardID {
			return true
		}
	}
	return false
}

func handleKinesisGetRecords(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ShardIterator string `json:"ShardIterator"`
		Limit         int    `json:"Limit"`
	}
	_ = sim.ReadJSON(r, &req)
	it, ok := kinesisIterators.Get(req.ShardIterator)
	if !ok {
		sim.AWSError(w, "ExpiredIteratorException", "Shard iterator expired", http.StatusBadRequest)
		return
	}
	records, _ := kinesisRecords.Get(kinesisShardRecordKey(it.StreamName, it.ShardID))
	limit := req.Limit
	if limit <= 0 || limit > 10000 {
		limit = 10000
	}
	end := it.Index + limit
	if end > len(records) {
		end = len(records)
	}
	out := make([]map[string]any, 0, end-it.Index)
	for _, rec := range records[it.Index:end] {
		out = append(out, map[string]any{
			"SequenceNumber":              rec.SequenceNumber,
			"ApproximateArrivalTimestamp": rec.ApproximateArrivalTimestamp,
			"Data":                        rec.Data,
			"PartitionKey":                rec.PartitionKey,
			"EncryptionType":              "NONE",
		})
	}
	next := generateUUID()
	kinesisIterators.Put(next, kinesisIterator{StreamName: it.StreamName, ShardID: it.ShardID, Index: end})
	writeKinesisJSON(w, http.StatusOK, map[string]any{
		"Records":            out,
		"NextShardIterator":  next,
		"MillisBehindLatest": 0,
	})
}

func handleKinesisAddTagsToStream(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StreamName string            `json:"StreamName"`
		StreamARN  string            `json:"StreamARN"`
		Tags       map[string]string `json:"Tags"`
	}
	_ = sim.ReadJSON(r, &req)
	stream, ok := kinesisStreamByNameOrARN(req.StreamName, req.StreamARN)
	if !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Stream not found", http.StatusBadRequest)
		return
	}
	if stream.Tags == nil {
		stream.Tags = map[string]string{}
	}
	for k, v := range req.Tags {
		stream.Tags[k] = v
	}
	kinesisStreams.Put(stream.StreamName, stream)
	writeKinesisJSON(w, http.StatusOK, map[string]any{})
}

func handleKinesisRemoveTagsFromStream(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StreamName string   `json:"StreamName"`
		StreamARN  string   `json:"StreamARN"`
		TagKeys    []string `json:"TagKeys"`
	}
	_ = sim.ReadJSON(r, &req)
	stream, ok := kinesisStreamByNameOrARN(req.StreamName, req.StreamARN)
	if !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Stream not found", http.StatusBadRequest)
		return
	}
	for _, key := range req.TagKeys {
		delete(stream.Tags, key)
	}
	kinesisStreams.Put(stream.StreamName, stream)
	writeKinesisJSON(w, http.StatusOK, map[string]any{})
}

func handleKinesisListTagsForStream(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StreamName string `json:"StreamName"`
		StreamARN  string `json:"StreamARN"`
	}
	_ = sim.ReadJSON(r, &req)
	stream, ok := kinesisStreamByNameOrARN(req.StreamName, req.StreamARN)
	if !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Stream not found", http.StatusBadRequest)
		return
	}
	keys := make([]string, 0, len(stream.Tags))
	for key := range stream.Tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	tags := make([]map[string]string, 0, len(keys))
	for _, key := range keys {
		tags = append(tags, map[string]string{"Key": key, "Value": stream.Tags[key]})
	}
	writeKinesisJSON(w, http.StatusOK, map[string]any{
		"Tags":        tags,
		"HasMoreTags": false,
	})
}

func handleKinesisIncreaseStreamRetentionPeriod(w http.ResponseWriter, r *http.Request) {
	kinesisUpdateRetention(w, r)
}

func handleKinesisDecreaseStreamRetentionPeriod(w http.ResponseWriter, r *http.Request) {
	kinesisUpdateRetention(w, r)
}

func kinesisUpdateRetention(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StreamName           string `json:"StreamName"`
		StreamARN            string `json:"StreamARN"`
		RetentionPeriodHours int64  `json:"RetentionPeriodHours"`
	}
	_ = sim.ReadJSON(r, &req)
	stream, ok := kinesisStreamByNameOrARN(req.StreamName, req.StreamARN)
	if !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Stream not found", http.StatusBadRequest)
		return
	}
	stream.RetentionPeriodHours = req.RetentionPeriodHours
	kinesisStreams.Put(stream.StreamName, stream)
	writeKinesisJSON(w, http.StatusOK, map[string]any{})
}

func handleKinesisEnableEnhancedMonitoring(w http.ResponseWriter, r *http.Request) {
	kinesisUpdateMonitoring(w, r, true)
}

func handleKinesisDisableEnhancedMonitoring(w http.ResponseWriter, r *http.Request) {
	kinesisUpdateMonitoring(w, r, false)
}

func kinesisUpdateMonitoring(w http.ResponseWriter, r *http.Request, enable bool) {
	var req struct {
		StreamName        string   `json:"StreamName"`
		StreamARN         string   `json:"StreamARN"`
		ShardLevelMetrics []string `json:"ShardLevelMetrics"`
	}
	_ = sim.ReadJSON(r, &req)
	stream, ok := kinesisStreamByNameOrARN(req.StreamName, req.StreamARN)
	if !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Stream not found", http.StatusBadRequest)
		return
	}
	current := []string{}
	if len(stream.EnhancedMonitoring) > 0 {
		if v, ok := stream.EnhancedMonitoring[0]["ShardLevelMetrics"].([]string); ok {
			current = append(current, v...)
		}
	}
	if enable {
		seen := map[string]bool{}
		for _, metric := range current {
			seen[metric] = true
		}
		for _, metric := range req.ShardLevelMetrics {
			if !seen[metric] {
				current = append(current, metric)
			}
		}
	} else {
		remove := map[string]bool{}
		for _, metric := range req.ShardLevelMetrics {
			remove[metric] = true
		}
		filtered := current[:0]
		for _, metric := range current {
			if !remove[metric] {
				filtered = append(filtered, metric)
			}
		}
		current = filtered
	}
	sort.Strings(current)
	stream.EnhancedMonitoring = []map[string]any{{"ShardLevelMetrics": current}}
	kinesisStreams.Put(stream.StreamName, stream)
	writeKinesisJSON(w, http.StatusOK, map[string]any{
		"StreamName":               stream.StreamName,
		"CurrentShardLevelMetrics": current,
		"DesiredShardLevelMetrics": current,
	})
}

func handleKinesisStartStreamEncryption(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StreamName     string `json:"StreamName"`
		StreamARN      string `json:"StreamARN"`
		EncryptionType string `json:"EncryptionType"`
		KeyId          string `json:"KeyId"`
	}
	_ = sim.ReadJSON(r, &req)
	stream, ok := kinesisStreamByNameOrARN(req.StreamName, req.StreamARN)
	if !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Stream not found", http.StatusBadRequest)
		return
	}
	stream.EncryptionType = req.EncryptionType
	stream.KeyId = req.KeyId
	kinesisStreams.Put(stream.StreamName, stream)
	writeKinesisJSON(w, http.StatusOK, map[string]any{})
}

func handleKinesisStopStreamEncryption(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StreamName string `json:"StreamName"`
		StreamARN  string `json:"StreamARN"`
	}
	_ = sim.ReadJSON(r, &req)
	stream, ok := kinesisStreamByNameOrARN(req.StreamName, req.StreamARN)
	if !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Stream not found", http.StatusBadRequest)
		return
	}
	stream.EncryptionType = ""
	stream.KeyId = ""
	kinesisStreams.Put(stream.StreamName, stream)
	writeKinesisJSON(w, http.StatusOK, map[string]any{})
}

func handleKinesisUpdateShardCount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StreamName       string `json:"StreamName"`
		StreamARN        string `json:"StreamARN"`
		TargetShardCount int64  `json:"TargetShardCount"`
	}
	_ = sim.ReadJSON(r, &req)
	stream, ok := kinesisStreamByNameOrARN(req.StreamName, req.StreamARN)
	if !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Stream not found", http.StatusBadRequest)
		return
	}
	current := int64(len(stream.Shards))
	stream.Shards = kinesisMakeShards(req.TargetShardCount)
	stream.OpenShardCount = req.TargetShardCount
	kinesisStreams.Put(stream.StreamName, stream)
	writeKinesisJSON(w, http.StatusOK, map[string]any{
		"StreamName":        stream.StreamName,
		"CurrentShardCount": current,
		"TargetShardCount":  req.TargetShardCount,
	})
}

func handleKinesisDescribeLimits(w http.ResponseWriter, r *http.Request) {
	open := 0
	for _, stream := range kinesisStreams.List() {
		open += len(stream.Shards)
	}
	writeKinesisJSON(w, http.StatusOK, map[string]any{
		"ShardLimit":               10000,
		"OpenShardCount":           open,
		"OnDemandStreamCount":      0,
		"OnDemandStreamCountLimit": 50,
	})
}
