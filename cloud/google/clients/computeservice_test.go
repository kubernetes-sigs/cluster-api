package clients_test

import (
	compute "google.golang.org/api/compute/v1"
	"encoding/json"
	"google.golang.org/api/googleapi"
	"net/http"
	"net/http/httptest"
	"sigs.k8s.io/cluster-api/cloud/google/clients"
	"testing"
)

func TestImagesGet(t *testing.T) {
	mux, server, client := createMuxServerAndClient()
	defer server.Close()
	responseImage := compute.Image{
		Name:             "imageName",
		ArchiveSizeBytes: 544,
	}
	mux.Handle("/compute/v1/projects/projectName/global/images/imageName", handler(nil, &responseImage))
	image, err := client.ImagesGet("projectName", "imageName")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if image == nil {
		t.Error("expected a valid image")
	}
	if "imageName" != image.Name {
		t.Errorf("expected imageName got %v", image.Name)
	}
	if image.ArchiveSizeBytes != int64(544) {
		t.Errorf("expected %v got %v", image.ArchiveSizeBytes, 544)
	}
}

func TestImagesGetFromFamily(t *testing.T) {
	mux, server, client := createMuxServerAndClient()
	defer server.Close()
	responseImage := compute.Image{
		Name:             "imageName",
		ArchiveSizeBytes: 544,
	}
	mux.Handle("/compute/v1/projects/projectName/global/images/family/familyName", handler(nil, &responseImage))
	image, err := client.ImagesGetFromFamily("projectName", "familyName")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if image == nil {
		t.Error("expected a valid image")
	}
	if "imageName" != image.Name {
		t.Errorf("expected imageName got %v", image.Name)
	}
	if image.ArchiveSizeBytes != int64(544) {
		t.Errorf("expected %v got %v", image.ArchiveSizeBytes, 544)
	}
}

func TestInstancesDelete(t *testing.T) {
	mux, server, client := createMuxServerAndClient()
	defer server.Close()
	responseOperation := compute.Operation{
		Id: 4501,
	}
	mux.Handle("/compute/v1/projects/projectName/zones/zoneName/instances/instanceName", handler(nil, &responseOperation))
	op, err := client.InstancesDelete("projectName", "zoneName", "instanceName")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if op == nil {
		t.Error("expected a valid operation")
	}
	if responseOperation.Id != uint64(4501) {
		t.Errorf("expected %v got %v", responseOperation.Id, 4501)
	}
}

func TestInstancesGet(t *testing.T) {
	mux, server, client := createMuxServerAndClient()
	defer server.Close()
	responseInstance := compute.Instance{
		Name: "instanceName",
		Zone: "zoneName",
	}
	mux.Handle("/compute/v1/projects/projectName/zones/zoneName/instances/instanceName", handler(nil, &responseInstance))
	instance, err := client.InstancesGet("projectName", "zoneName", "instanceName")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if instance == nil {
		t.Error("expected a valid instance")
	}
	if "instanceName" != instance.Name {
		t.Errorf("expected instanceName got %v", instance.Name)
	}
	if "zoneName" != instance.Zone {
		t.Errorf("expected zoneName got %v", instance.Zone)
	}
}

func TestInstancesInsert(t *testing.T) {
	mux, server, client := createMuxServerAndClient()
	defer server.Close()
	responseOperation := compute.Operation{
		Id: 3001,
	}
	mux.Handle("/compute/v1/projects/projectName/zones/zoneName/instances", handler(nil, &responseOperation))
	op, err := client.InstancesInsert("projectName", "zoneName", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if op == nil {
		t.Error("expected a valid operation")
	}
	if responseOperation.Id != uint64(3001) {
		t.Errorf("expected %v got %v", responseOperation.Id, 3001)
	}
}

func createMuxServerAndClient() (*http.ServeMux, *httptest.Server, *clients.ComputeService) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	client, _ := clients.NewComputeServiceForURL(server.Client(), server.URL)
	return mux, server, client
}

func handler(err *googleapi.Error, obj interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		handleTestRequest(w, err, obj)
	}
}

func handleTestRequest(w http.ResponseWriter, handleErr *googleapi.Error, obj interface{}) {
	if handleErr != nil {
		http.Error(w, errMsg(handleErr), handleErr.Code)
		return
	}
	res, err := json.Marshal(obj)
	if err != nil {
		http.Error(w, "json marshal error", http.StatusInternalServerError)
		return
	}
	w.Write(res)
}

func errMsg(e *googleapi.Error) string {
	res, err := json.Marshal(&errorReply{e})
	if err != nil {
		return "json marshal error"
	}
	return string(res)
}

type errorReply struct {
	Error *googleapi.Error `json:"error"`
}
