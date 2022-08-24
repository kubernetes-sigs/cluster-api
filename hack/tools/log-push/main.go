//go:build tools
// +build tools

/*
Copyright 2022 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// main is the main package for Log Push.
package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/pkg/errors"
	"github.com/valyala/fastjson"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"k8s.io/klog/v2"
)

var (
	logPath                 = flag.String("log-path", "", "Can be either a GCS path, a ProwJob URL or a local directory")
	logFileRegex            = flag.String("log-file-regex", "manager\\.log", "Regex used to find log files")
	logJSONAdditionalLabels = flag.String("log-json-additional-labels", "controller,cluster,machine", "Comma-separated list of additional labels to parse from JSON logs")
	lokiURL                 = flag.String("loki-url", "http://localhost:3100/loki/api/v1/push", "Loki URL to push the logs to")
)

func main() {
	flag.Parse()
	if *logPath == "" {
		fmt.Println("--log-path must be set")
		os.Exit(1)
	}
	logFileRegexp, err := regexp.Compile(*logFileRegex)
	if err != nil {
		fmt.Println("--log-file-regex must be a valid regex")
		os.Exit(1)
	}
	logJSONAdditionalLabelsArray := strings.Split(*logJSONAdditionalLabels, ",")

	if err := importLogs(*logPath, logFileRegexp, logJSONAdditionalLabelsArray, *lokiURL); err != nil {
		fmt.Printf("Failed to import logs: %v\n", err)
		os.Exit(1)
	}

	klog.Infof("Finished syncing logs from %s", *logPath)
}

func importLogs(logPath string, logFileRegex *regexp.Regexp, logJSONAdditionalLabels []string, lokiURL string) error {
	ctx := context.Background()

	// Get Logs.
	logs, err := getLogs(ctx, logPath, logFileRegex)
	if err != nil {
		return errors.Wrapf(err, "failed to get logs")
	}

	for logFile, logData := range logs {
		// Prepare logs for Loki.
		streams, err := prepareLogsForLoki(logData, logJSONAdditionalLabels)
		if err != nil {
			return errors.Wrapf(err, "failed to prepare logs for Loki")
		}

		// Push logs to Loki.
		if err := pushLogsToLoki(ctx, lokiURL, logFile, streams); err != nil {
			return errors.Wrapf(err, "failed to push logs to Loki")
		}
	}

	return nil
}

// LogData represents the log file and corresponding metadata.
type LogData struct {
	logs     []byte
	metadata []byte
	fileName string
}

// getLogs gets logs either from GCS or local files.
func getLogs(ctx context.Context, logPath string, logFileRegex *regexp.Regexp) (map[string]LogData, error) {
	if !strings.HasPrefix(logPath, "gs://") && !strings.HasPrefix(logPath, "https://") {
		return getLogsFromFile(logPath, logFileRegex)
	}

	return getLogsFromGCS(ctx, logPath, logFileRegex)
}

// getLogsFromFile gets logs from file.
func getLogsFromFile(logPath string, logFileRegex *regexp.Regexp) (map[string]LogData, error) {
	klog.Infof("Getting logs from %s", logPath)

	logData := map[string]LogData{}
	err := filepath.Walk(logPath, func(fileName string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !logFileRegex.MatchString(fileName) {
			return nil
		}

		ld := LogData{
			fileName: fileName,
		}

		file, err := os.ReadFile(fileName) //nolint:gosec // file inclusion via variable is not an issue here.
		if err != nil {
			return errors.Wrapf(err, "failed to read log file from filesystem")
		}
		ld.logs = file

		// Try to read the corresponding metadata file from filesystem (e.g. manager.log => manager-log-metadata.json).
		metadataFileName := strings.Replace(fileName, ".log", "-log-metadata.json", 1)
		metadataFile, err := os.ReadFile(metadataFileName) //nolint:gosec // file inclusion via variable is not an issue here.
		if err != nil {
			klog.Warningf("Adding logs without file metadata, failed to read log metadata file %q from filesystem: %v", metadataFileName, err)
		} else {
			ld.metadata = metadataFile
		}

		logData[fileName] = ld
		return nil
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read logs from filesystem")
	}

	return logData, nil
}

// getLogsFromGCS gets logs from GCS.
func getLogsFromGCS(ctx context.Context, logPath string, logFileRegex *regexp.Regexp) (map[string]LogData, error) {
	// Calculate GCS log location based on either a GCS path or a ProwJob URL.
	bucket, folder, err := calculateGCSLogLocation(logPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get logs from GCS")
	}

	klog.Infof("Getting logs from gs://%s/%s", bucket, folder)

	// Set timeout.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	// Create GCS client.
	client, err := storage.NewClient(ctx, option.WithoutAuthentication())
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %v", err)
	}
	defer client.Close()

	// Create iterator to iterate over GCS files in folder.
	query := &storage.Query{
		Prefix:    folder,
		Delimiter: "",
	}
	it := client.Bucket(bucket).Objects(ctx, query)

	// Iterate over files and collect LogData.
	logData := map[string]LogData{}
	for {
		attrs, err := it.Next()
		if err != nil {
			// Break if there are no other files.
			if err == iterator.Done {
				break
			}
			return nil, errors.Wrapf(err, "failed to get logs from GCS: failed to get next file")
		}
		fileName := attrs.Name

		// Continue if the current file does not match the log file regex.
		if !logFileRegex.MatchString(fileName) {
			continue
		}

		ld := LogData{
			fileName: fmt.Sprintf("gs://%s/%s", bucket, attrs.Name),
		}

		// Download manager.log file from GCS.
		file, err := downloadFileFromGCS(ctx, client, bucket, fileName)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get log file from GCS")
		}
		ld.logs = file

		// Try to download corresponding metadata file from GCS (e.g. manager.log => manager-log-metadata.json).
		metadataFileName := strings.Replace(fileName, ".log", "-log-metadata.json", 1)
		metadataFile, err := downloadFileFromGCS(ctx, client, bucket, metadataFileName)
		if err != nil {
			klog.Warningf("Adding logs without file metadata, failed to get log metadata file %q from GCS: %v", metadataFileName, err)
		} else {
			ld.metadata = metadataFile
		}

		logData[ld.fileName] = ld
	}

	return logData, nil
}

// calculateGCSLogLocation calculates GCS log location based on either a GCS path or a ProwJob URL.
func calculateGCSLogLocation(logPath string) (string, string, error) {
	u, err := url.Parse(logPath)
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to parse log path %q", logPath)
	}

	if u.Scheme == "https" {
		// Valid ProwJob URL:
		// https://prow.k8s.io/view/gs/kubernetes-jenkins/pr-logs/pull/kubernetes-sigs_cluster-api/6189/pull-cluster-api-build-main/1496233282759561216
		// u.Path then is: /view/gs/kubernetes-jenkins/pr-logs/pull/kubernetes-sigs_cluster-api/6189/pull-cluster-api-build-main/1496233282759561216
		pathSegments := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
		if len(pathSegments) < 4 {
			return "", "", errors.Wrapf(err, "failed to parse log path %q: unexpected format", logPath)
		}

		bucket := pathSegments[2]
		folder := path.Join(pathSegments[3:]...)
		return bucket, folder, nil
	}

	// Valid gs URL:
	// gs://kubernetes-jenkins/pr-logs/pull/kubernetes-sigs_cluster-api/6189/pull-cluster-api-build-main/1496233282759561216
	// u.Host then is: kubernetes-jenkins
	// u.Path then is: /pr-logs/pull/kubernetes-sigs_cluster-api/6189/pull-cluster-api-build-main/1496233282759561216
	bucket := u.Host
	folder := strings.TrimPrefix(u.Path, "/")
	return bucket, folder, nil
}

// downloadFileFromGCS downloads a file from GCS.
func downloadFileFromGCS(ctx context.Context, client *storage.Client, bucket, object string) ([]byte, error) {
	klog.Infof("Downloading gs://%s/%s", bucket, object)

	// Create reader for object.
	rc, err := client.Bucket(bucket).Object(object).NewReader(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create reader for file %q", object)
	}
	defer rc.Close()

	// Read object.
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read file %q", object)
	}
	return data, nil
}

// LokiStreams represents streams pushed to Loki.
type LokiStreams struct {
	Streams []LokiStream `json:"streams"`
}

// LokiStream represents a stream pushed to Loki.
type LokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

// prepareLogsForLoki builds LokiStreams for the push to Loki based on LogData.
// See: https://grafana.com/docs/loki/latest/api/#post-lokiapiv1push.
func prepareLogsForLoki(ld LogData, logJSONAdditionalLabels []string) ([]LokiStreams, error) {
	metadata := map[string]string{}

	// Add metadata from metadata file if possible.
	if len(ld.metadata) > 0 {
		if err := json.Unmarshal(ld.metadata, &metadata); err != nil {
			return nil, err
		}
	}
	// Add file name.
	metadata["filename"] = ld.fileName

	allStreams := []LokiStream{}
	for _, logLine := range strings.Split(string(ld.logs), "\n") {
		// SKip if the log line is empty.
		if logLine == "" {
			continue
		}

		// If we use the original timestamps, it can take 5 to 10 minutes for the logs to show up in Loki.
		// So we are using the current timestamp for now.
		tsNano := strconv.FormatInt(time.Now().UnixNano(), 10)

		// Copy the metadata into logLineMetadata.
		logLineMetadata := map[string]string{}
		for k, v := range metadata {
			logLineMetadata[k] = v
		}

		parsedLogLine, err := fastjson.Parse(logLine)
		// We intentionally silently ignore the error, otherwise we
		// would get too many errors with logs in text format.
		if err == nil {
			// Store the ts in original_ts so that it's shown as a separate k/v in Loki.
			if parsedLogLine.Exists("ts") {
				originalTimestampMillis := parsedLogLine.Get("ts")
				parsedLogLine.Set("original_tsMs", originalTimestampMillis)

				t := time.UnixMilli(int64(originalTimestampMillis.GetFloat64()))
				originalTimestamp := strconv.Quote(t.Format(time.RFC3339Nano))
				parsedLogLine.Set("original_ts", fastjson.MustParse(originalTimestamp))

				// Overwrite the original log line with the one with the additional original timestamp.
				logLine = parsedLogLine.String()
			}

			// Add cluster and machine labels to logLineMetadata
			// if they exist in the current log line.
			for _, label := range logJSONAdditionalLabels {
				if !parsedLogLine.Exists(label) {
					continue
				}

				labelValue := parsedLogLine.Get(label).String()
				labelValue, err = strconv.Unquote(labelValue)
				if labelValue == "" {
					continue
				}
				if err != nil {
					return nil, errors.Wrapf(err, "failed to unquote label %q: %q", label, labelValue)
				}
				logLineMetadata[label] = labelValue
			}
		}

		allStreams = append(allStreams, LokiStream{
			Stream: logLineMetadata,
			Values: [][]string{{tsNano, logLine}},
		})
	}

	// We have to batch the streams, because we can only push
	// up to 4 MB with one HTTP request to Loki.
	batchSize := 1000
	batchedStreams := []LokiStreams{}
	for i := 0; i < len(allStreams); i += batchSize {
		l := LokiStreams{
			Streams: allStreams[i:min(i+batchSize, len(allStreams))],
		}
		batchedStreams = append(batchedStreams, l)
	}

	return batchedStreams, nil
}

// pushLogsToLoki uploads data to Loki.
func pushLogsToLoki(ctx context.Context, lokiURL, file string, lokiLogStreamsArray []LokiStreams) error {
	klog.Infof("Pushing logs to Loki from: %s", file)

	for _, streams := range lokiLogStreamsArray {
		// Marshal streams to JSON.
		body, err := json.Marshal(streams)
		if err != nil {
			return errors.Wrapf(err, "failed to marshal Loki stream")
		}

		if err := pushStreamToLoki(ctx, lokiURL, body); err != nil {
			return errors.Wrapf(err, "failed to push Loki stream")
		}
	}

	return nil
}

func pushStreamToLoki(ctx context.Context, lokiURL string, body []byte) error {
	// gzip JSON into buf.
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(body); err != nil {
		return errors.Wrapf(err, "failed to gzip Loki data: failed to write")
	}
	if err := gz.Close(); err != nil {
		return errors.Wrapf(err, "failed to gzip Loki data: failed to close writer")
	}

	// Create the request.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, lokiURL, &buf)
	if err != nil {
		return errors.Wrapf(err, "failed to create request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrapf(err, "failed to do request")
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrapf(err, "failed to read response")
	}

	klog.Infof("Push response: status: %q, body: %q", resp.Status, string(respBody))
	return nil
}

func min(a, b int) int {
	if a <= b {
		return a
	}
	return b
}
