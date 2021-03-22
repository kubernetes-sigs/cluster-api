/*
Copyright The Kubernetes Authors.

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

// Code generated for package config by go-bindata DO NOT EDIT. (@generated)
// sources:
// cmd/clusterctl/config/manifest/clusterctl-api.yaml
package config

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func bindataRead(data []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("Read %q: %v", name, err)
	}

	var buf bytes.Buffer
	_, err = io.Copy(&buf, gz)
	clErr := gz.Close()

	if err != nil {
		return nil, fmt.Errorf("Read %q: %v", name, err)
	}
	if clErr != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

type asset struct {
	bytes []byte
	info  os.FileInfo
}

type bindataFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
}

// Name return file name
func (fi bindataFileInfo) Name() string {
	return fi.name
}

// Size return file size
func (fi bindataFileInfo) Size() int64 {
	return fi.size
}

// Mode return file mode
func (fi bindataFileInfo) Mode() os.FileMode {
	return fi.mode
}

// Mode return file modify time
func (fi bindataFileInfo) ModTime() time.Time {
	return fi.modTime
}

// IsDir return file whether a directory
func (fi bindataFileInfo) IsDir() bool {
	return fi.mode&os.ModeDir != 0
}

// Sys return file is sys mode
func (fi bindataFileInfo) Sys() interface{} {
	return nil
}

var _cmdClusterctlConfigManifestClusterctlApiYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\xb4\x56\x4d\x8f\x1b\x37\x0f\xbe\xfb\x57\x10\x79\x0f\xb9\xbc\x1e\x27\x08\x0a\x14\x73\x2b\xb6\x3d\x04\xfd\xc0\x22\xbb\xd8\x1c\x8a\x1e\x64\x89\xb6\x99\xd5\x50\xaa\x48\x39\x71\x8b\xfe\xf7\x42\x92\xc7\x9e\xf1\xb6\x8e\x2f\x9d\x93\xf5\x88\x1f\x8f\x1e\x91\x94\x4d\xa4\x27\x4c\x42\x81\x7b\x30\x91\xf0\x8b\x22\x97\x95\x74\xcf\xdf\x4a\x47\x61\xb5\x7f\xbb\x78\x26\x76\x3d\xdc\x65\xd1\x30\x7c\x40\x09\x39\x59\xfc\x1e\x37\xc4\xa4\x14\x78\x31\xa0\x1a\x67\xd4\xf4\x0b\x00\xc3\x1c\xd4\x14\x58\xca\x12\xc0\x06\xd6\x14\xbc\xc7\xb4\xdc\x22\x77\xcf\x79\x8d\xeb\x4c\xde\x61\xaa\xc1\xc7\xd4\xfb\x37\xdd\x37\xdd\x9b\x05\x80\x4d\x58\xdd\x1f\x69\x40\x51\x33\xc4\x1e\x38\x7b\xbf\x00\x60\x33\x60\x0f\x31\x85\x3d\x39\x4c\xd2\x59\x9f\x45\x31\x59\xf5\xe3\xcf\xee\xcb\xb2\x91\x5e\x48\x44\x5b\xf2\x6f\x53\xc8\xb1\x87\x6b\xa6\x2d\xf0\xc8\xd6\x28\x6e\x43\xa2\x71\xbd\x1c\x5d\x97\x26\x52\x45\x9a\x16\xf7\x47\x16\x15\xf2\x24\xfa\xe3\x0c\xfe\x89\x44\xeb\x56\xf4\x39\x19\x3f\x61\x5d\x51\x21\xde\x66\x6f\xd2\x19\x5f\x00\x88\x0d\x11\x7b\xf8\xa5\x90\x89\xc6\xa2\x5b\x00\x1c\xe5\xa9\x64\x96\x60\x9c\xab\x82\x1b\x7f\x9f\x88\x15\xd3\x5d\xf0\x79\xe0\x13\xd5\x4f\x12\xf8\xde\xe8\xae\x87\x4e\x0f\x11\x2b\x3a\xca\xf6\x78\x06\xca\x5e\x0f\xa2\x89\x78\xfb\xd2\x73\x64\x54\x78\xcc\x22\xcc\x8e\xfc\xb5\x28\x47\xe2\xb3\x00\x4f\x33\xec\xba\xff\x67\xa3\x76\x87\xee\x24\xc6\x2c\xd0\xc7\xb2\x09\x97\x7b\x2f\x02\x36\xe3\xfd\x5b\xe3\xe3\xce\xbc\x6b\xc2\xdb\x1d\x0e\xa6\x3f\x7a\x84\x88\xfc\xdd\xfd\xfb\xa7\x77\x0f\x33\x18\xc0\xa1\xd8\x44\x51\x6b\x65\x8e\xe7\x06\x57\x2a\x1e\x05\x0c\x03\xb2\xa6\x03\x10\x83\xee\xf0\x74\x87\x40\xbc\x47\xd6\x90\x0e\xdd\x29\x52\x4c\x21\x62\xd2\x53\x3d\xb5\x6f\xd2\x73\x13\xf4\x22\xef\xeb\x42\xad\x59\x9d\x52\x97\x74\x47\x69\xd1\x1d\x4f\x03\x61\x03\xba\x23\x81\x84\x31\xa1\x20\xb7\xf6\x2b\xb0\x61\x08\xeb\x4f\x68\xb5\x83\x07\x4c\xc5\x11\x64\x17\xb2\x77\xa5\x2b\xf7\x98\x14\x12\xda\xb0\x65\xfa\xe3\x14\x4d\x40\x43\x4d\xe3\x8d\xa2\x28\xd4\x3a\x63\xe3\x61\x6f\x7c\xc6\xff\x83\x61\x07\x83\x39\x40\xc2\x12\x17\x32\x4f\x22\x54\x13\xe9\xe0\xe7\x90\x10\x88\x37\xa1\x87\x9d\x6a\x94\x7e\xb5\xda\x92\x8e\xf3\xc4\x86\x61\xc8\x4c\x7a\x58\xd5\xd1\x40\xeb\xac\x21\xc9\xca\xe1\x1e\xfd\x4a\x68\xbb\x34\xc9\xee\x48\xd1\x6a\x4e\xb8\x32\x91\x96\x95\x2c\xd7\x99\xd2\x0d\xee\x7f\xe9\x38\x81\xe4\xf5\x4c\xbc\x17\xf7\xdf\xbe\xda\xaf\x57\x54\x2e\x8d\x0b\x24\x60\x8e\xae\xed\x14\x67\x31\x0b\x54\xf4\xf8\xf0\xc3\xc3\x23\x8c\xa9\x9b\xe0\x4d\xdb\xb3\xa9\x9c\x65\x2e\x12\x11\x6f\x30\x35\xcb\x4d\x0a\x43\x8d\x82\xec\x62\x20\xd6\xba\xb0\x9e\x90\x15\x24\xaf\x07\xd2\x72\x7f\xbf\x67\x14\x2d\x37\xd0\xc1\x5d\x1d\xa4\xb0\x46\xc8\xd1\x19\x45\xd7\xc1\x7b\x86\x3b\x33\xa0\xbf\x33\x82\xff\xb9\xc8\x45\x4d\x59\x16\xf1\x6e\x93\x79\xfa\x06\x5c\x1a\x37\x9d\x26\x1b\xd3\x19\x73\xe5\x6e\xee\x27\x66\x40\xec\xa8\x4c\xe7\xd6\x04\xa5\xb7\x5b\xe1\x9f\xfb\xaf\xbb\x85\x67\x85\xff\x3d\x65\x19\x93\x17\xa9\x8a\xc7\x8b\x54\xf0\x80\x78\xe2\x57\x9d\x36\x21\x81\xa9\x4f\x41\x31\x96\x1c\x63\x48\x7a\x6a\x8a\x5b\xa8\xed\xbf\x3a\x12\xc6\x71\x30\x27\x68\xc3\x10\x03\x97\x4a\x3a\x46\xb8\x49\x88\xcb\x09\x7b\x25\xed\xc7\x0b\xd3\x7f\xb8\x8b\x86\x7f\xde\x61\xc2\xf9\x4c\x3c\x3f\xff\xa5\xc9\x48\x5a\x5e\xe2\x6d\x07\xb4\x01\x1c\xa2\x1e\xae\x39\x8c\xd6\x55\xde\x56\x47\x52\xe6\xae\xf1\xfe\x9c\x57\x6e\x38\xf0\x8b\x4a\x94\xd2\xa9\xae\x07\x4d\xb9\xbd\x20\xa2\x21\x99\x2d\x4e\x91\xbc\x3e\xcd\x9a\x1e\xfe\xfc\x6b\x21\x6a\x34\xd7\x49\x6e\xac\xc5\xa8\x47\x4d\xfa\xc9\x1f\x83\x57\xaf\x66\xef\x7e\x5d\xda\xc0\xed\xe1\x96\x1e\x7e\xfd\x6d\xd1\x52\xa1\x7b\x1a\x1f\xf7\x02\xfe\x1d\x00\x00\xff\xff\xb4\x73\xf0\x1a\x87\x09\x00\x00")

func cmdClusterctlConfigManifestClusterctlApiYamlBytes() ([]byte, error) {
	return bindataRead(
		_cmdClusterctlConfigManifestClusterctlApiYaml,
		"cmd/clusterctl/config/manifest/clusterctl-api.yaml",
	)
}

func cmdClusterctlConfigManifestClusterctlApiYaml() (*asset, error) {
	bytes, err := cmdClusterctlConfigManifestClusterctlApiYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "cmd/clusterctl/config/manifest/clusterctl-api.yaml", size: 2439, mode: os.FileMode(420), modTime: time.Unix(1, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

// Asset loads and returns the asset for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func Asset(name string) ([]byte, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		a, err := f()
		if err != nil {
			return nil, fmt.Errorf("Asset %s can't read by error: %v", name, err)
		}
		return a.bytes, nil
	}
	return nil, fmt.Errorf("Asset %s not found", name)
}

// MustAsset is like Asset but panics when Asset would return an error.
// It simplifies safe initialization of global variables.
func MustAsset(name string) []byte {
	a, err := Asset(name)
	if err != nil {
		panic("asset: Asset(" + name + "): " + err.Error())
	}

	return a
}

// AssetInfo loads and returns the asset info for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func AssetInfo(name string) (os.FileInfo, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		a, err := f()
		if err != nil {
			return nil, fmt.Errorf("AssetInfo %s can't read by error: %v", name, err)
		}
		return a.info, nil
	}
	return nil, fmt.Errorf("AssetInfo %s not found", name)
}

// AssetNames returns the names of the assets.
func AssetNames() []string {
	names := make([]string, 0, len(_bindata))
	for name := range _bindata {
		names = append(names, name)
	}
	return names
}

// _bindata is a table, holding each asset generator, mapped to its name.
var _bindata = map[string]func() (*asset, error){
	"cmd/clusterctl/config/manifest/clusterctl-api.yaml": cmdClusterctlConfigManifestClusterctlApiYaml,
}

// AssetDir returns the file names below a certain
// directory embedded in the file by go-bindata.
// For example if you run go-bindata on data/... and data contains the
// following hierarchy:
//     data/
//       foo.txt
//       img/
//         a.png
//         b.png
// then AssetDir("data") would return []string{"foo.txt", "img"}
// AssetDir("data/img") would return []string{"a.png", "b.png"}
// AssetDir("foo.txt") and AssetDir("notexist") would return an error
// AssetDir("") will return []string{"data"}.
func AssetDir(name string) ([]string, error) {
	node := _bintree
	if len(name) != 0 {
		cannonicalName := strings.Replace(name, "\\", "/", -1)
		pathList := strings.Split(cannonicalName, "/")
		for _, p := range pathList {
			node = node.Children[p]
			if node == nil {
				return nil, fmt.Errorf("Asset %s not found", name)
			}
		}
	}
	if node.Func != nil {
		return nil, fmt.Errorf("Asset %s not found", name)
	}
	rv := make([]string, 0, len(node.Children))
	for childName := range node.Children {
		rv = append(rv, childName)
	}
	return rv, nil
}

type bintree struct {
	Func     func() (*asset, error)
	Children map[string]*bintree
}

var _bintree = &bintree{nil, map[string]*bintree{
	"cmd": &bintree{nil, map[string]*bintree{
		"clusterctl": &bintree{nil, map[string]*bintree{
			"config": &bintree{nil, map[string]*bintree{
				"manifest": &bintree{nil, map[string]*bintree{
					"clusterctl-api.yaml": &bintree{cmdClusterctlConfigManifestClusterctlApiYaml, map[string]*bintree{}},
				}},
			}},
		}},
	}},
}}

// RestoreAsset restores an asset under the given directory
func RestoreAsset(dir, name string) error {
	data, err := Asset(name)
	if err != nil {
		return err
	}
	info, err := AssetInfo(name)
	if err != nil {
		return err
	}
	err = os.MkdirAll(_filePath(dir, filepath.Dir(name)), os.FileMode(0755))
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(_filePath(dir, name), data, info.Mode())
	if err != nil {
		return err
	}
	err = os.Chtimes(_filePath(dir, name), info.ModTime(), info.ModTime())
	if err != nil {
		return err
	}
	return nil
}

// RestoreAssets restores an asset under the given directory recursively
func RestoreAssets(dir, name string) error {
	children, err := AssetDir(name)
	// File
	if err != nil {
		return RestoreAsset(dir, name)
	}
	// Dir
	for _, child := range children {
		err = RestoreAssets(dir, filepath.Join(name, child))
		if err != nil {
			return err
		}
	}
	return nil
}

func _filePath(dir, name string) string {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	return filepath.Join(append([]string{dir}, strings.Split(cannonicalName, "/")...)...)
}
