// Author: Phil Roche - phil.roche@chainguard.dev
// Date: 20240808
// This is a simple CLI tool that lists the latest version of a given package across all wolfi repositories
// The tool takes a list of package names as arguments and returns the latest version of each package
// The tool also supports regex matching of package names
// The tool also supports the `--help` flag to display usage information
// The tool uses the go-humanize library to format the build time of the package
package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/dustin/go-humanize"
	"github.com/knqyf263/go-apk-version"
	"gitlab.alpinelinux.org/alpine/go/repository"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"
)

func isValidRegex(pattern string) bool {
	_, err := regexp.Compile(pattern)
	return err == nil
}

func matchRegex(s string, pattern string) bool {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		log.Fatalf("Invalid regex pattern: %v", err)
	}
	return regex.MatchString(s)
}

func getEnvOrFlag(envName string, flagValue *string) string {
	if value, exists := os.LookupEnv(envName); exists {
		return value
	}
	return *flagValue
}

func removeDuplicates(stringsList []string) []string {
	seen := make(map[string]struct{})
	var result []string

	for _, item := range stringsList {
		if _, found := seen[item]; !found {
			seen[item] = struct{}{}
			result = append(result, item)
		}
	}

	return result
}

// Version represents a single version entry in the JSON
type Version struct {
	BuildTime  time.Time `json:"BuildTime"`
	Origin     string    `json:"Origin"`
	Repository string    `json:"Repository"`
	Version    string    `json:"Version"`
}

// PackageInfo represents the overall structure for a package
type PackageInfo struct {
	Versions    []Version `json:"versions"`
	SubPackages []string  `json:"subpackages"`
}

// Result represents the top-level structure for handling multiple packages
type Result map[string]PackageInfo
type SubPackages map[string][]string

func main() {
	matchAsRegex := flag.Bool("regex", false, "Parse package names as regex")
	listAllVersions := flag.Bool("all-versions", false, "List all matching package versions - not only the latest")
	localAPKINDEX := flag.String("local-apkindex", "", "Path to a local APKINDEX file")
	localAuthToken := flag.String("auth-token", "", "Specify auth token to use when querying non public wolfi package repositories - enterprise-packages and extra-packages - use $(chainctl auth token --audience apk.cgr.dev). You can also set environment variable HTTP_AUTH.")
	showParentPackageInformation := flag.Bool("show-parent-package", false, "This might be a sub package of a parent package, show the parent package information")
	showSubPackageInformation := flag.Bool("show-sub-packages", false, "Show the sub package information. This will only take effect when a non regex package name filter is used.")
	helpText := flag.Bool("help", false, "Display usage information")
	outputJSON := flag.Bool("json", false, "Render output in JSON format")
	flag.Parse()
	httpBasicAuthPassword := getEnvOrFlag("HTTP_AUTH", localAuthToken)
	if httpBasicAuthPassword == "" {
		fmt.Print("Specifying an auth token is required. Use `chainctl auth token --audience apk.cgr.dev` to get the required token. Please enter token now - alternatively, you can also specify this via --auth-token flag or by setting HTTP_AUTH environment variable: ")
		_, _ = fmt.Scanln(&httpBasicAuthPassword)
	}
	if *helpText {
		fmt.Printf("Usage: %s [options] [package names]\n", os.Args[0])
		fmt.Println("\t* Multiple package names can be specified separated by space")
		fmt.Println("\t* Option `--regex` can be used to match package names on specified regular expression. Multiple regular expressions can be specified separated by space")
		fmt.Println("\t* Option `--all-versions` can be used to list all package versions, not only the latest.")
		fmt.Println("\t* Option `--auth-token` specify auth token to use when querying wolfi non public package repositories - enterprise-packages and extra-packages - use $(chainctl auth token --audience apk.cgr.dev). You can also set environment variable HTTP_AUTH.")
		fmt.Println("\t* Option `--show-parent-package` can be used to show the parent package information as the package being queried might be defined as a sub package.")
		fmt.Println("\t* Option `--show-sub-packages` can be used to show the sub package information as the package being queried might be defined as a parent/origin package. This will only take effect when a non regex package name filter is used.")
		fmt.Println("\t* Option `--local-apkindex` can be used to specify a local APKINDEX.tar.gz file to use instead of querying remote repositories.")
		fmt.Println("\t* Option `--json` can be used to render output in JSON format.")
		fmt.Println("\t* Option `--help` can be used to display this usage message")
		os.Exit(0)
	}

	queryStrings := []string{}
	args := flag.Args()
	if len(args) > 0 {
		for _, arg := range args {
			queryStrings = append(queryStrings, arg)
		}
	}

	var APKINDEXURLs = make(map[string]string)

	if *localAPKINDEX != "" {
		APKINDEXURLs["local apkindex"] = *localAPKINDEX
	} else {
		APKINDEXURLs["wolfi os"] = "https://packages.wolfi.dev/os/x86_64/APKINDEX.tar.gz"
		APKINDEXURLs["enterprise packages"] = "https://apk.cgr.dev/chainguard-private/x86_64/APKINDEX.tar.gz"
		APKINDEXURLs["extra packages"] = "https://apk.cgr.dev/extra-packages/x86_64/APKINDEX.tar.gz"
	}

	// Create the Result map
	results := Result{}
	subPackages := SubPackages{}

	var wg sync.WaitGroup
	var resultsMutex sync.Mutex
	var subPackagesMutex sync.Mutex
	resultsChan := make(chan Result)
	subPackagesChan := make(chan SubPackages)
	errorsChan := make(chan error)

	var receiverWg sync.WaitGroup

	// Goroutine to read from resultsChan
	receiverWg.Add(1)
	go func() {
		defer receiverWg.Done()
		for _resultsChan := range resultsChan {
			for _packageName, _pkgInfo := range _resultsChan {
				resultsMutex.Lock()
				if _, exists := results[_packageName]; exists {
					pkgInfoTemp := results[_packageName]
					pkgInfoTemp.Versions = append(pkgInfoTemp.Versions, _pkgInfo.Versions...)
					results[_packageName] = pkgInfoTemp
				} else {
					results[_packageName] = _pkgInfo
				}
				resultsMutex.Unlock()
			}
		}

	}()

	receiverWg.Add(1)
	go func() {
		defer receiverWg.Done()
		for _subPackagesChan := range subPackagesChan {
			for _packageName, _subPackages := range _subPackagesChan {
				subPackagesMutex.Lock()
				subPackages[_packageName] = append(subPackages[_packageName], _subPackages...)
				subPackagesMutex.Unlock()
			}
		}

	}()

	receiverWg.Add(1)
	go func() {
		defer receiverWg.Done()
		for _errorsChan := range errorsChan {
			fmt.Println("Encountered errors:", _errorsChan)
		}

	}()

	//for each of the APKINDEXURLs create an instance of the repository class
	for APKINDEXFriendlyName, APKINDEXurl := range APKINDEXURLs {
		wg.Add(1)
		go func(_friendlyName, _apkIndexURL string) {
			defer wg.Done()
			// check to see of APKINDEXurl is a local file
			localAPKINDEXPath := ""
			temporaryAPKINDEXdir := ""
			if _, err := os.Stat(_apkIndexURL); err == nil {
				localAPKINDEXPath = _apkIndexURL
			} else {
				// Download each of the APKINDEX files to temporary directory using "net/http"

				// use localAuthToken if it is set when making request to non public repositories
				// Create a new request
				req, err := http.NewRequest("GET", _apkIndexURL, nil)
				if err != nil {
					fmt.Println("Error creating request:", err)
					return
				}

				// Add the auth token to the request header but only for non public repositories
				if httpBasicAuthPassword != "" && _friendlyName != "wolfi os" {
					encodedAuth := base64.StdEncoding.EncodeToString([]byte("user:" + httpBasicAuthPassword))
					req.Header.Set("Authorization", "Basic "+encodedAuth)
				}

				req.Header.Set("Accept", "application/gzip")
				req.Header.Add("User-Agent", "curl/7.68.0")

				// Send the request via a client
				client := &http.Client{}
				resp, err := client.Do(req)
				if err != nil {
					fmt.Printf("Failed to download APKINDEX file %s: %v\n", _apkIndexURL, err)
					os.Exit(1)
				}

				// write the response variable resp to a file in a temporary directory
				defer resp.Body.Close()
				// Create a temporary directory
				temporaryAPKINDEXdir, err = os.MkdirTemp("", "wolfi-package-status")
				if err != nil {
					fmt.Printf("Failed to create temporary directory %s: %v\n", temporaryAPKINDEXdir, err)
					os.Exit(1)
				}

				// Create a file in the temporary directory
				localAPKINDEXPath = filepath.Join(temporaryAPKINDEXdir, "APKINDEX.tar.gz")
				localAPKINDEXfile, err := os.Create(localAPKINDEXPath)
				if err != nil {
					fmt.Printf("Failed to write APKINDEX file to temporary directory %s: %v\n", temporaryAPKINDEXdir, err)
					os.Exit(1)
				}
				defer localAPKINDEXfile.Close()

				// Write the response to file
				_, err = io.Copy(localAPKINDEXfile, resp.Body)
			}
			indexFile, err := os.Open(localAPKINDEXPath)
			if err != nil {
				errorsChan <- fmt.Errorf("failed to open APKINDEX file %s: %v", localAPKINDEXPath, err)
				return
			}
			defer indexFile.Close()
			apkIndex, err := repository.IndexFromArchive(indexFile)
			if err != nil {
				errorsChan <- fmt.Errorf("failed to read APKINDEX archive %s: %v", localAPKINDEXPath, err)
				return
			}
			packages := apkIndex.Packages
			localResults := Result{}
			localSubPackages := SubPackages{}

			for _, _package := range packages {
				var matchFound bool

				if len(queryStrings) > 0 {
					for _, queryString := range queryStrings {
						matchFound = false
						if queryString == _package.Name {
							matchFound = true
						}
						if *matchAsRegex && isValidRegex(queryString) && matchRegex(_package.Name, queryString) {
							matchFound = true
						}

						if matchFound {
							pkgVersion := Version{
								BuildTime:  _package.BuildTime,
								Origin:     _package.Origin,
								Repository: _friendlyName,
								Version:    _package.Version,
							}

							// Create a PackageInfo structure for each package
							pkgInfo := PackageInfo{
								Versions:    []Version{},
								SubPackages: []string{},
							}
							// Append the version to the Versions slice
							pkgInfo.Versions = append(pkgInfo.Versions, pkgVersion)

							if _, exists := localResults[_package.Name]; exists {
								pkgInfoTemp := localResults[_package.Name]
								pkgInfoTemp.Versions = append(pkgInfoTemp.Versions, pkgVersion)
								localResults[_package.Name] = pkgInfoTemp
							} else {
								// Add the PackageInfo to the Result map
								localResults[_package.Name] = pkgInfo
							}

						}
						//Gather all subpackage names
						if _package.Origin != "" && _package.Origin != _package.Name {
							localSubPackages[_package.Origin] = append(localSubPackages[_package.Origin], _package.Name)
						}

					}
				} else {
					// we are not matching any packages here so print all found package names and versions
					_parentPackageInformation := ""
					if *showParentPackageInformation {
						_parentPackageInformation = " - Parent/Origin package: " + _package.Origin
					}
					fmt.Printf("%s version %s (%s - %s) in %s repository%s\n", _package.Name, _package.Version, humanize.Time(_package.BuildTime), _package.BuildTime, APKINDEXFriendlyName, _parentPackageInformation)
				}
			}
			if localAPKINDEXPath != _apkIndexURL {
				// delete the temporary directory
				err = os.RemoveAll(temporaryAPKINDEXdir)
				if err != nil {
					fmt.Printf("Unable to delete temporary directory %s: %v\n", temporaryAPKINDEXdir, err)
					os.Exit(1)
				}
			}

			resultsChan <- localResults
			subPackagesChan <- localSubPackages
		}(APKINDEXFriendlyName, APKINDEXurl)

	}

	// Close the results and errors channels once all goroutines are done
	wg.Wait()
	close(resultsChan)
	close(subPackagesChan)
	close(errorsChan)

	// Wait for all receiver goroutines to complete
	receiverWg.Wait()

	// now sort the versions within the per package info
	for _packageName, _pkgInfo := range results {
		if len(_pkgInfo.Versions) > 1 {
			_versions := _pkgInfo.Versions
			sort.Slice(_versions, func(i, j int) bool {
				v1, _ := version.NewVersion(_versions[i].Version)
				v2, _ := version.NewVersion(_versions[j].Version)
				return v2.GreaterThan(v1) // Sort in ascending order (earliest first)
			})
			_pkgInfo.Versions = _versions
			packageSubPackagesIncludingDuplicates := subPackages[_packageName]
			_pkgInfo.SubPackages = removeDuplicates(packageSubPackagesIncludingDuplicates)
			results[_packageName] = _pkgInfo
		}
	}

	if !*listAllVersions {
		// Loop through all the packages and delete all versions apart from last index
		for _packageName, _pkgInfo := range results {
			if len(_pkgInfo.Versions) > 1 {
				_pkgInfo.Versions = _pkgInfo.Versions[len(_pkgInfo.Versions)-1:]
				results[_packageName] = _pkgInfo
			}
		}
	}

	if *outputJSON {
		jsonOutputBytes := []byte{}
		err := error(nil)

		jsonOutputBytes, err = json.MarshalIndent(results, "", "  ")
		if err != nil {
			log.Fatalf("Error marshalling JSON: %v", err)
		}
		fmt.Println(string(jsonOutputBytes))
	} else {
		// sort the results by package name
		_packageNameKeys := make([]string, 0, len(results))
		for key := range results {
			_packageNameKeys = append(_packageNameKeys, key)
		}
		sort.Strings(_packageNameKeys) // Sort the keys alphabetically

		for _, key := range _packageNameKeys {
			_packageName := key
			_pkgInfo := results[key]
			fmt.Printf("The versions of package %s are:\n", _packageName)
			_parentPackageInformation := ""

			for _, _version := range _pkgInfo.Versions {
				if *showParentPackageInformation {
					_parentPackageInformation = " - Parent/Origin package: " + _version.Origin
				}
				fmt.Printf("\t%s (%s - %s) in %s repository%s\n", _version.Version, humanize.Time(_version.BuildTime), _version.BuildTime, _version.Repository, _parentPackageInformation)
			}
			if *showSubPackageInformation && !*matchAsRegex && len(_pkgInfo.SubPackages) > 0 {
				fmt.Printf("\tSub packages:\n")
				for _, subPackageName := range _pkgInfo.SubPackages {
					fmt.Printf("\t\t%s\n", subPackageName)
				}
			}
		}
	}
}
