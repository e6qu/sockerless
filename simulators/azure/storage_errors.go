package main

import (
	"encoding/xml"
	"net/http"
)

type storageErrorResponse struct {
	XMLName xml.Name `xml:"Error"`
	Code    string   `xml:"Code"`
	Message string   `xml:"Message"`
}

func writeStorageError(w http.ResponseWriter, code, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("x-ms-error-code", code)
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(storageErrorResponse{
		Code:    code,
		Message: message,
	})
}

func writeStorageXML(w http.ResponseWriter, statusCode int, v any) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(v)
}

type storageServiceProperties struct {
	XMLName       xml.Name              `xml:"StorageServiceProperties"`
	Logging       storageLogging        `xml:"Logging"`
	HourMetrics   storageMetrics        `xml:"HourMetrics"`
	MinuteMetrics storageMetrics        `xml:"MinuteMetrics"`
	Cors          storageCorsProperties `xml:"Cors"`
}

type storageLogging struct {
	Version         string                 `xml:"Version"`
	Delete          bool                   `xml:"Delete"`
	Read            bool                   `xml:"Read"`
	Write           bool                   `xml:"Write"`
	RetentionPolicy storageRetentionPolicy `xml:"RetentionPolicy"`
}

type storageMetrics struct {
	Version         string                 `xml:"Version"`
	Enabled         bool                   `xml:"Enabled"`
	IncludeAPIs     bool                   `xml:"IncludeAPIs"`
	RetentionPolicy storageRetentionPolicy `xml:"RetentionPolicy"`
}

type storageRetentionPolicy struct {
	Enabled bool `xml:"Enabled"`
}

type storageCorsProperties struct{}

func defaultStorageServiceProperties() storageServiceProperties {
	return storageServiceProperties{
		Logging: storageLogging{
			Version:         "1.0",
			RetentionPolicy: storageRetentionPolicy{},
		},
		HourMetrics: storageMetrics{
			Version:         "1.0",
			RetentionPolicy: storageRetentionPolicy{},
		},
		MinuteMetrics: storageMetrics{
			Version:         "1.0",
			RetentionPolicy: storageRetentionPolicy{},
		},
	}
}
