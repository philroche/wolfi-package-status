package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	version "github.com/knqyf263/go-apk-version"
	"gitlab.alpinelinux.org/alpine/go/repository"
	"golang.org/x/exp/maps"
)

type APKIndex string

var (
	WOLFI               APKIndex = "wolfi os"
	ENTERPRISE_PACKAGES APKIndex = "enterprise packages"
	EXTRA_PACKAGES      APKIndex = "extra packages"
	LOCAL               APKIndex = "local"

	DefaultAPKIndices = map[APKIndex]string{
		WOLFI:               "https://packages.wolfi.dev/os/x86_64/APKINDEX.tar.gz",
		ENTERPRISE_PACKAGES: "https://apk.cgr.dev/chainguard-private/x86_64/APKINDEX.tar.gz",
		EXTRA_PACKAGES:      "https://apk.cgr.dev/extra-packages/x86_64/APKINDEX.tar.gz",
	}
)

// PackageMeta represents a single package version entry in the JSON
type PackageMeta struct {
	BuildTime  time.Time `json:"BuildTime"`
	Origin     string    `json:"Origin"`
	Repository string    `json:"Repository"`
	Version    string    `json:"Version"`
}

// SubPackageMeta represents a single subpackage version entry in the JSON
type SubPackageMeta struct {
	PackageMeta `json:",inline"`
	Name        string `json:"name"`
}

// PackageData represents the overall structure for a package
type PackageData struct {
	Versions    []PackageMeta    `json:"versions"`
	SubPackages []SubPackageMeta `json:"subpackages"`
}

// Output represents the top-level structure for handling multiple packages
type PackageInfoOutput struct {
	m      sync.Mutex
	Result map[string]PackageData `json:",inline"`
}

func (p *PackageInfoOutput) AddPackageMeta(q []Matcher, pkgmeta *repository.Package, repo string) {
	isMatched := false
	isSubPkg := false
	// if the package matches the queries then it directly gets added as a package
	// if the package doesn't match any query but is a subpackage of a matched package, then,
	// it only gets added as a subpackage
	if matchReference(q, pkgmeta.Name) {
		if len(pkgmeta.Origin) != 0 && matchReference(q, pkgmeta.Origin) {
			isSubPkg = true
			isMatched = false
		} else {
			return
		}
	} else {
		isMatched = true
	}

	pkgVersion := PackageMeta{
		BuildTime:  pkgmeta.BuildTime,
		Origin:     pkgmeta.Origin,
		Repository: repo,
		Version:    pkgmeta.Version,
	}

	p.m.Lock()
	defer p.m.Unlock()
	if isMatched {
		if _, exists := p.Result[pkgmeta.Name]; exists {
			pkgInfoTemp := p.Result[pkgmeta.Name]
			pkgInfoTemp.Versions = append(pkgInfoTemp.Versions, pkgVersion)
			p.Result[pkgmeta.Name] = pkgInfoTemp
		} else {
			pkgInfo := PackageData{
				Versions:    []PackageMeta{pkgVersion},
				SubPackages: []SubPackageMeta{},
			}
			p.Result[pkgmeta.Name] = pkgInfo
		}
	}

	if isSubPkg {
		subPkgInfo := SubPackageMeta{
			PackageMeta: pkgVersion,
			Name:        pkgmeta.Name,
		}

		if _, exists := p.Result[pkgmeta.Origin]; exists {
			pkgInfoTemp := p.Result[pkgmeta.Origin]
			pkgInfoTemp.SubPackages = append(pkgInfoTemp.SubPackages, subPkgInfo)
			p.Result[pkgmeta.Origin] = pkgInfoTemp
		} else {
			pkgInfo := PackageData{
				Versions:    []PackageMeta{},
				SubPackages: []SubPackageMeta{subPkgInfo},
			}
			p.Result[pkgmeta.Origin] = pkgInfo
		}
	}

	return
}

// Sort sorts all the versions of a packages in ascending order
func (p *PackageInfoOutput) Sort() {
	pkgInfo := p.Result
	for _, pkgData := range pkgInfo {
		if len(pkgData.Versions) > 1 {
			versions := pkgData.Versions
			sort.Slice(versions, func(i, j int) bool {
				v1, _ := version.NewVersion(versions[i].Version)
				v2, _ := version.NewVersion(versions[j].Version)
				return v2.GreaterThan(v1) // Sort in ascending order (earliest first)
			})
			pkgData.Versions = versions

		}
		if len(pkgData.SubPackages) > 1 {
			subpkg := pkgData.SubPackages
			sort.Slice(subpkg, func(i, j int) bool {
				name1 := subpkg[i].Name
				name2 := subpkg[j].Name
				n := strings.Compare(name1, name2)
				if n != 0 {
					return n == -1
				}

				v1, _ := version.NewVersion(subpkg[i].Version)
				v2, _ := version.NewVersion(subpkg[j].Version)
				return v2.GreaterThan(v1) // Sort in ascending order (earliest first)
			})
			pkgData.SubPackages = subpkg
		}
	}
	p.Result = pkgInfo
}

func (p *PackageInfoOutput) Print(listAll, printJSON, showParentPkgInfo, showSubPkgInfo bool) {
	results := p.Result
	if !listAll {
		// Loop through all the packages and delete all versions apart from last index
		for name, pkgInfo := range results {
			if len(pkgInfo.Versions) > 1 {
				pkgInfo.Versions = pkgInfo.Versions[len(pkgInfo.Versions)-1:]
				results[name] = pkgInfo
			}
			if len(pkgInfo.SubPackages) > 0 && len(pkgInfo.Versions) == 1 {
				repo := pkgInfo.Versions[0].Repository
				ver := pkgInfo.Versions[0].Version
				var subpkg []SubPackageMeta
				for _, s := range pkgInfo.SubPackages {
					if s.Version == ver && s.Repository == repo {
						subpkg = append(subpkg, s)
					}
				}
				pkgInfo.SubPackages = subpkg
			}
		}
	}

	if printJSON {
		jsonOutputBytes := []byte{}
		var err error

		jsonOutputBytes, err = json.MarshalIndent(results, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshalling JSON: %v", err)
			os.Exit(1)
		}
		fmt.Println(string(jsonOutputBytes))
	} else {
		// sort the results by package name
		mapKeys := maps.Keys(p.Result)
		sort.Strings(mapKeys) // Sort the keys alphabetically

		for _, key := range mapKeys {
			pkgInfo := results[key]
			fmt.Fprintf(os.Stdout, "The versions of package %s are:\n", key)

			for _, v := range pkgInfo.Versions {
				parentPkgInfo := ""
				if showParentPkgInfo {
					parentPkgInfo = " - Parent/Origin package: " + v.Origin
				}
				fmt.Fprintf(os.Stdout, "\t%s (%s - %s) in %s repository%s\n", v.Version, humanize.Time(v.BuildTime), v.BuildTime, v.Repository, parentPkgInfo)
			}
			if showSubPkgInfo && len(pkgInfo.SubPackages) > 0 {
				fmt.Printf("\tSub packages:\n")
				for _, s := range pkgInfo.SubPackages {
					fmt.Fprintf(os.Stdout, "\t%s %s (%s - %s) in %s repository\n", s.Name, s.Version, humanize.Time(s.BuildTime), s.BuildTime, s.Repository)
				}
			}
		}
	}
}
