package objects

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/streaming"
	"k8s.io/kubernetes/pkg/kubectl/scheme"
)

// getCRDs should actually use kustomize to correctly build the manager yaml.
// HACK: this is a hacked function
func getCAPIYAML(version, capiImage string) (io.Reader, error) {
	crds := []string{"crds", "rbac", "manager"}
	releaseCode := fmt.Sprintf("https://github.com/kubernetes-sigs/cluster-api/archive/%s.tar.gz", version)

	resp, err := http.Get(releaseCode)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	tgz := tar.NewReader(gz)
	var buf bytes.Buffer

	for {
		header, err := tgz.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, errors.WithStack(err)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
			for _, crd := range crds {
				// Skip the kustomization files for now. Would like to use kustomize in future
				if strings.HasSuffix(header.Name, "kustomization.yaml") {
					continue
				}

				// This is a poor person's kustomize
				if strings.HasSuffix(header.Name, "manager.yaml") {
					var managerBuf bytes.Buffer
					io.Copy(&managerBuf, tgz)
					lines := strings.Split(managerBuf.String(), "\n")
					for _, line := range lines {
						if strings.Contains(line, "image:") {
							buf.WriteString(strings.Replace(line, "image: controller:latest", fmt.Sprintf("image: %s", capiImage), 1))
							buf.WriteString("\n")
							continue
						}
						buf.WriteString(line)
						buf.WriteString("\n")
					}
				}

				// These files don't need kustomize at all.
				if strings.Contains(header.Name, fmt.Sprintf("config/%s/", crd)) {
					io.Copy(&buf, tgz)
					fmt.Fprintln(&buf, "---")
				}
			}
		}
	}
	return &buf, nil
}

func getCAPIObjects(yaml io.Reader) ([]runtime.Object, error) {
	decoder := scheme.Codecs.UniversalDeserializer()
	streamingDecoder := streaming.NewDecoder(ioutil.NopCloser(yaml), decoder)

	defer streamingDecoder.Close()

	objects := []runtime.Object{}
	for {
		obj, _, err := streamingDecoder.Decode(nil, nil)
		if err == io.EOF {
			break
		}

		if err != nil {
			return []runtime.Object{}, errors.Wrap(err, "couldn't decode CAPI object")
		}

		objects = append(objects, obj)
	}

	return objects, nil
}

func GetCAPI(version, capiImage string) ([]runtime.Object, error) {
	reader, err := getCAPIYAML(version, capiImage)
	if err != nil {
		return []runtime.Object{}, err
	}

	return getCAPIObjects(reader)
}
