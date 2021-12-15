/*
Copyright 2021 The Kubernetes Authors.

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
	"go/types"
	"path"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"
	crdmarkers "sigs.k8s.io/controller-tools/pkg/crd/markers"
	"sigs.k8s.io/controller-tools/pkg/loader"
	"sigs.k8s.io/controller-tools/pkg/markers"
)

var (
	groupNameMarker        = markers.Must(markers.MakeDefinition("groupName", markers.DescribesPackage, ""))
	storageVersionMarker   = markers.Must(markers.MakeDefinition("kubebuilder:storageversion", markers.DescribesType, crdmarkers.StorageVersion{}))
	nolintConversionMarker = markers.Must(markers.MakeDefinition("kubebuilder:conversion:nolint", markers.DescribesType, false))

	apiPackages         = map[string]*apiPackage{}
	storageVersionTypes = map[string]*storageVersionType{}
)

type apiPackage struct {
	*loader.Package
	Group string
}

// versionType contains information about
// a specific type.
type versionType struct {
	pkg *loader.Package
	typ *markers.TypeInfo
}

// IsConvertible checks if the type has ConvertFrom and ConvertTo methods.
func (v *versionType) IsConvertible() bool {
	return hasMethod(v.pkg, v.typ, "ConvertFrom") && hasMethod(v.pkg, v.typ, "ConvertTo")
}

// storageVersionType is a versionType that has been found to be also a storage version
// this type should only be created once per type.
type storageVersionType struct {
	*versionType

	// otherDecls contains all the other declarations for the same type in other packages.
	otherDecls map[string]*versionType
}

// IsHub checks if the type has Hub method.
func (s *storageVersionType) IsHub() bool {
	return hasMethod(s.pkg, s.typ, "Hub")
}

func main() {
	var result error

	// Define the marker collector.
	col := &markers.Collector{
		Registry: &markers.Registry{},
	}
	// Register the markers.
	if err := col.Registry.Register(groupNameMarker); err != nil {
		klog.Fatal(err)
	}
	if err := col.Registry.Register(storageVersionMarker); err != nil {
		klog.Fatal(err)
	}
	if err := col.Registry.Register(nolintConversionMarker); err != nil {
		klog.Fatal(err)
	}

	// Load all packages.
	// TODO: Move the path parameter to a multi-string flag.
	packages, err := loader.LoadRoots("./...")
	if err != nil {
		klog.Fatal(err)
	}

	// First loop through all the packages
	// and find the ones that have a storage version assigned.
	for _, pkg := range packages {
		group := apiGroupForPackage(col, pkg)
		if group == "" {
			// We're only interested in api folders, exclude everything
			// else that's not an API package.
			continue
		}

		// Store the packages.
		apiPackages[pkg.PkgPath] = &apiPackage{
			Package: pkg,
			Group:   group,
		}

		// Populate type information and loop through every type in from this package root.
		pkg.NeedTypesInfo()
		if err := markers.EachType(col, pkg, func(info *markers.TypeInfo) {
			// Check if the type is registered with storage version.
			if m := info.Markers.Get(storageVersionMarker.Name); m == nil {
				return
			}
			if _, ok := storageVersionTypes[info.Name]; ok {
				result = multierror.Append(result,
					errors.Errorf("type %q has a redeclared storage version in package %q", info.Name, pkg.PkgPath),
				)
				return
			}

			// Store the type and package as selected storage version.
			storageVersionTypes[path.Join(group, info.Name)] = &storageVersionType{
				versionType: &versionType{
					pkg: pkg,
					typ: info,
				},
				otherDecls: make(map[string]*versionType),
			}
		}); err != nil {
			klog.Fatal(err)
		}
	}

	// Find and populate all <Kind>List types.
	for _, pkg := range apiPackages {
		if err := markers.EachType(col, pkg.Package, func(info *markers.TypeInfo) {
			if !strings.HasSuffix(info.Name, "List") {
				// We're only interested in List types in this iteration.
				return
			}
			storage, ok := storageVersionTypes[path.Join(pkg.Group, strings.TrimSuffix(info.Name, "List"))]
			if !ok || pkg.PkgPath != storage.pkg.PkgPath {
				// Return early if the type isn't in the same storage package
				// or if there is no storage version registered for this type.
				return
			}

			// If we're here, we've found <Kind>List type that's also a storage version.
			storageVersionTypes[path.Join(pkg.Group, info.Name)] = &storageVersionType{
				versionType: &versionType{
					pkg: pkg.Package,
					typ: info,
				},
				otherDecls: make(map[string]*versionType),
			}
		}); err != nil {
			klog.Fatal(err)
		}
	}

	// Now that we have storage version information,
	// loop through every package again and collect info about all declarations.
	for _, pkg := range apiPackages {
		if err := markers.EachType(col, pkg.Package, func(info *markers.TypeInfo) {
			storage, ok := storageVersionTypes[path.Join(pkg.Group, info.Name)]
			if !ok {
				// If this type isn't a storage version, keep looping.
				return
			}
			if pkg.PkgPath == storage.pkg.PkgPath {
				// If we're in the same package, we've probably found again the
				// storage version.
				return
			}
			// Check if the type should not be checked against conversion rules.
			if m := info.Markers.Get(nolintConversionMarker.Name); m != nil && m.(bool) {
				return
			}
			storage.otherDecls[pkg.PkgPath] = &versionType{
				pkg: pkg.Package,
				typ: info,
			}
		}); err != nil {
			klog.Fatal(err)
		}
	}

	for name, storageType := range storageVersionTypes {
		if len(storageType.otherDecls) == 0 {
			continue
		}

		if !storageType.IsHub() {
			result = multierror.Append(result,
				errors.Errorf("type %q in package %q marked as storage version but it's not convertible, missing Hub() method", name, storageType.pkg.PkgPath),
			)
		}
		for _, decl := range storageType.otherDecls {
			if !decl.IsConvertible() {
				result = multierror.Append(result,
					errors.Errorf("type %q in package %q it's not convertible, missing ConvertFrom() and ConvertTo() methods", name, decl.pkg.PkgPath),
				)
			}
		}
	}

	if result != nil {
		klog.Exit(result.Error())
	}
}

func apiGroupForPackage(col *markers.Collector, pkg *loader.Package) string {
	pkgMarkers, err := markers.PackageMarkers(col, pkg)
	if err != nil {
		klog.Fatal(err)
	}
	group := pkgMarkers.Get(groupNameMarker.Name)
	if group == nil {
		return ""
	}
	return group.(string)
}

func hasMethod(pkg *loader.Package, typ *markers.TypeInfo, name string) bool {
	t := pkg.TypesInfo.TypeOf(typ.RawSpec.Name)
	method, ind, _ := types.LookupFieldOrMethod(t, true, pkg.Types, name)
	if len(ind) != 1 {
		// ignore embedded methods
		return false
	}
	if method == nil {
		return false
	}
	return true
}
