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

package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
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
	bucket           = flag.String("log-bucket", "kubernetes-jenkins", "Bucket to download the logs from")
	controllerFolder = flag.String("log-controller-folder", "", "Folder to get the controller-logs from")
	lokiURL          = flag.String("loki-url", "http://localhost:3100/loki/api/v1/push", "URL to push the logs to")
)

func main() {
	flag.Parse()
	if *controllerFolder == "" {
		fmt.Printf("--log-controller-folder must be set\n")
	}

	if err := run(); err != nil {
		fmt.Printf("Error occured: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	// Create GCS client.
	client, err := storage.NewClient(ctx, option.WithoutAuthentication())
	if err != nil {
		return fmt.Errorf("failed to create GCS client: %v", err)
	}
	defer client.Close()

	// Get logs from GCS
	logs, err := getLogsFromGCS(ctx, client, *bucket, *controllerFolder)
	if err != nil {
		return errors.Wrapf(err, "error getting logs from GCS")
	}

	for logDir, ld := range logs {
		streams, err := prepareDataForLoki(ld)
		if err != nil {
			return errors.Wrapf(err, "failed to prepare data for Loki")
		}

		if err := uploadDataToLoki(logDir, streams); err != nil {
			return errors.Wrapf(err, "failed to upload data to Loki")
		}
	}

	return nil
}

type LogData struct {
	logs     []byte
	metadata []byte
}

// getLogsFromGCS gets logs from GCS.
func getLogsFromGCS(ctx context.Context, client *storage.Client, bucket, controllerFolder string) (map[string]LogData, error) {
	// Get iterator to iterate over GCS files in controllerFolder.
	query := &storage.Query{
		Prefix:    controllerFolder,
		Delimiter: "",
	}
	it := client.Bucket(bucket).Objects(ctx, query)

	// Iterate over files and collect LogData.
	data := map[string]LogData{}
	for {
		attrs, err := it.Next()
		if err != nil {
			// Break if there is no next file.
			if err == iterator.Done {
				break
			}
			return nil, errors.Wrapf(err, "failed to get next file")
		}

		// Continue if we found a file other than manager.log or manager.json
		if !strings.HasSuffix(attrs.Name, "manager.log") && !strings.HasSuffix(attrs.Name, "manager.json") {
			continue
		}

		// Use the parent directory of manager.{log|json} as log data key.
		dir := attrs.Name[:strings.LastIndex(attrs.Name, "/")]
		ld, _ := data[dir]

		// Download file from GCS.
		file, err := downloadFileFromGCS(ctx, client, bucket, attrs.Name)
		if err != nil {
			return nil, err
		}

		// Store file either as logs or metadata.
		if strings.HasSuffix(attrs.Name, "manager.log") {
			ld.logs = file
		} else {
			ld.metadata = file
		}

		data[dir] = ld
	}

	return data, nil
}

// downloadFileFromGCS downloads a file from GCS.
func downloadFileFromGCS(ctx context.Context, client *storage.Client, bucket, object string) ([]byte, error) {
	// Set timeout.
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	// Creat reader for object.
	rc, err := client.Bucket(bucket).Object(object).NewReader(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create reader for file %q", object)
	}
	defer rc.Close()

	// Read object.
	data, err := ioutil.ReadAll(rc)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read file %q", object)
	}
	return data, nil
}

type LokiLogStreams struct {
	Streams []LokiStream `json:"streams"`
}

type LokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

// prepareDataForLoki builds LokiLogStreams for upload to Loki based on LogData.
// See: https://grafana.com/docs/loki/latest/api/#post-lokiapiv1push.
func prepareDataForLoki(ld LogData) (*LokiLogStreams, error) {
	// Unmarshal the fileMetadata.
	fileMetadata := map[string]string{}
	if err := json.Unmarshal(ld.metadata, &fileMetadata); err != nil {
		return nil, err
	}

	streams := LokiLogStreams{}
	for _, logLine := range strings.Split(string(ld.logs), "\n") {
		parsedLogLine, err := fastjson.Parse(logLine)
		if err != nil {
			fmt.Printf("Failed to parse log line %s: %v\n", logLine, err)
			continue
		}

		// Copy the fileMetadata into logLineMetadata.
		logLineMetadata := map[string]string{}
		for k, v := range fileMetadata {
			logLineMetadata[k] = v
		}

		// Add cluster and machine labels to logLineMetadata
		// if they exist in the current log line.
		additionalLabels := []string{"cluster", "machine"}
		for _, label := range additionalLabels {
			if parsedLogLine.Exists(label) {
				labelValue := parsedLogLine.Get(label).String()
				labelValue, err = strconv.Unquote(labelValue)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to unquote label %q: %q", label, labelValue)
				}
				logLineMetadata[label] = labelValue
			}
		}

		// If we use the original timestamps, it can take 5 to 10 minutes for the logs to show up in Loki.
		// So we are using the current timestamp for now.
		tsNano := strconv.FormatInt(time.Now().UnixNano(), 10)

		streams.Streams = append(streams.Streams, LokiStream{
			Stream: logLineMetadata,
			Values: [][]string{{tsNano, logLine}},
		})

	}
	return &streams, nil
}

// uploadDataToLoki uploads data to Loki.
func uploadDataToLoki(logDir string, streams *LokiLogStreams) error {
	klog.Infof("Uploading logs from: %s", logDir)

	// Marshal streams to JSON.
	body, err := json.Marshal(streams)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal Loki data")
	}

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
	req, err := http.NewRequest(http.MethodPost, *lokiURL, &buf)
	if err != nil {
		return errors.Wrapf(err, "failed to create request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrapf(err, "failed to do request")
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrapf(err, "failed to read response")
	}
	defer resp.Body.Close()

	klog.Infof("Response: status: %q, body: %q", resp.Status, string(respBody))

	return nil
}
