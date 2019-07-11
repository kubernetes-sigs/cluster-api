package objects

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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

func decodeCAPIObjects(yaml io.Reader) ([]runtime.Object, error) {
	decoder := scheme.Codecs.UniversalDeserializer()
	objects := []runtime.Object{}
	readbuf := bufio.NewReader(yaml)
	writebuf := &bytes.Buffer{}

	for {
		line, err := readbuf.ReadBytes('\n')
		// End of an object, parse it
		if err == io.EOF || bytes.Equal(line, []byte("---\n")) {

			// Use unstructured because scheme may not know about CRDs
			if writebuf.Len() > 1 {
				obj, _, err := decoder.Decode(writebuf.Bytes(), nil, &unstructured.Unstructured{})
				if err == nil {
					objects = append(objects, obj)
				} else {
					return []runtime.Object{}, errors.Wrap(err, "couldn't decode CAPI object")
				}
			}

			// previously we didn't care if this was EOF or ---, but now we need to break the loop
			if err == io.EOF {
				break
			}

			// No matter what happened, start over
			writebuf.Reset()
		} else if err != nil {
			return []runtime.Object{}, errors.Wrap(err, "couldn't read YAML")
		} else {
			// Just an ordinary line
			writebuf.Write(line)
		}
	}

	return objects, nil
}

func GetCAPI(version, capiImage string) ([]runtime.Object, error) {
	reader, err := getCAPIYAML(version, capiImage)
	if err != nil {
		return []runtime.Object{}, err
	}

	return decodeCAPIObjects(reader)
}
