package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func testSetup() (cleanup, reset func()) {
	wStream := WriteStream
	eStream := ErrorStream
	defaultIndices := DefaultAPKIndices

	var writeBuffer bytes.Buffer
	var errorBuffer bytes.Buffer

	WriteStream = &writeBuffer
	ErrorStream = &errorBuffer

	DefaultAPKIndices = map[APKIndex]string{
		WOLFI: "https://packages.wolfi.dev/os/x86_64/APKINDEX.tar.gz",
	}

	return func() {
			WriteStream = wStream
			ErrorStream = eStream
			DefaultAPKIndices = defaultIndices
		},
		func() {
			writeBuffer.Reset()
			errorBuffer.Reset()
		}
}

func Test_MainExecute(t *testing.T) {
	cleanupEnv, resetEnv := testSetup()
	defer cleanupEnv()

	tests := []struct {
		name              string
		queries           []string
		regex             bool
		listAllVersions   bool
		showParentPkgInfo bool
		showSubPkgInfo    bool
		outputJSON        bool
		verifyJSON        func(map[string]PackageData) error

		errStreamContains   []string
		writeStreamContains []string
	}{
		{
			name:                "single package",
			queries:             []string{"python-3.13"},
			listAllVersions:     false,
			showParentPkgInfo:   true,
			showSubPkgInfo:      true,
			outputJSON:          false,
			writeStreamContains: []string{"The versions of package python-3.13 are:", "Parent/Origin package: python-3.13", "Sub packages:", "python-3.13-base"},
		},
		{
			name:              "multiple packages",
			queries:           []string{"python-3.12", "python-3.13"},
			listAllVersions:   false,
			showParentPkgInfo: true,
			showSubPkgInfo:    true,
			outputJSON:        false,
			writeStreamContains: []string{
				"The versions of package python-3.12 are:", "Parent/Origin package: python-3.12", "Sub packages:", "python-3.12-base",
				"The versions of package python-3.13 are:", "Parent/Origin package: python-3.13", "Sub packages:", "python-3.13-base",
			},
		},
		{
			name:              "regex single package",
			queries:           []string{"python-3.13.*"},
			regex:             true,
			listAllVersions:   false,
			showParentPkgInfo: true,
			showSubPkgInfo:    true,
			outputJSON:        false,
			writeStreamContains: []string{
				"The versions of package python-3.13 are:",
				"The versions of package python-3.13-tk are:",
				"The versions of package python-3.13-base are:",
				"The versions of package python-3.13-dev are:",
				"The versions of package python-3.13-doc are:",
				"The versions of package python-3.13-base-dev are:",
				"The versions of package python-3.13-privileged-netbindservice are:",
			},
		},
		{
			name:              "regex multiple packages",
			queries:           []string{"python-3.12.*", "python-3.13.*"},
			regex:             true,
			listAllVersions:   false,
			showParentPkgInfo: true,
			showSubPkgInfo:    true,
			outputJSON:        false,
			writeStreamContains: []string{
				// python 3.12
				"The versions of package python-3.12 are:",
				"The versions of package python-3.12-tk are:",
				"The versions of package python-3.12-base are:",
				"The versions of package python-3.12-dev are:",
				"The versions of package python-3.12-doc are:",
				"The versions of package python-3.12-base-dev are:",
				"The versions of package python-3.12-privileged-netbindservice are:",
				// python 3.13
				"The versions of package python-3.13 are:",
				"The versions of package python-3.13-tk are:",
				"The versions of package python-3.13-base are:",
				"The versions of package python-3.13-dev are:",
				"The versions of package python-3.13-doc are:",
				"The versions of package python-3.13-base-dev are:",
				"The versions of package python-3.13-privileged-netbindservice are:",
			},
		},
		{
			name:              "json single package",
			queries:           []string{"python-3.13"},
			listAllVersions:   false,
			showParentPkgInfo: true,
			showSubPkgInfo:    true,
			outputJSON:        true,
			verifyJSON: func(m map[string]PackageData) error {
				py3_13, exists := m["python-3.13"]
				if !exists {
					return fmt.Errorf("python 3.13 not found")
				}

				if len(py3_13.Versions) != 1 {
					return fmt.Errorf("python 3.13 printed more version when list all version is false")
				}

				if len(py3_13.SubPackages) == 0 {
					return fmt.Errorf("python 3.13 does not have subpackages")
				}

				for _, subpkg := range []string{
					"python-3.13-base",
					"python-3.13-base-dev",
					"python-3.13-dev",
					"python-3.13-doc",
					"python-3.13-privileged-netbindservice",
					"python-3.13-tk",
				} {
					if !slices.Contains(py3_13.SubPackages, subpkg) {
						return fmt.Errorf("python 3.13 does not have sub package %s", subpkg)
					}
				}
				return nil
			},
		},
		{
			name:              "json multiple packages",
			queries:           []string{"python-3.12", "python-3.13"},
			listAllVersions:   false,
			showParentPkgInfo: true,
			showSubPkgInfo:    true,
			outputJSON:        true,
			verifyJSON: func(m map[string]PackageData) error {
				if len(m) != 2 {
					return fmt.Errorf("expected map length 2, found %d map: %+v", len(m), m)
				}
				_, exists := m["python-3.13"]
				if !exists {
					return fmt.Errorf("python 3.13 not found")
				}

				py3_12, exists := m["python-3.12"]
				if !exists {
					return fmt.Errorf("python 3.12 not found")
				}

				if len(py3_12.Versions) != 1 {
					return fmt.Errorf("python 3.12 printed more version when list all version is false")
				}

				if len(py3_12.SubPackages) == 0 {
					return fmt.Errorf("python 3.12 does not have subpackages")
				}

				for _, subpkg := range []string{
					"python-3.12-base",
					"python-3.12-base-dev",
					"python-3.12-dev",
					"python-3.12-doc",
					"python-3.12-privileged-netbindservice",
					"python-3.12-tk",
				} {
					if !slices.Contains(py3_12.SubPackages, subpkg) {
						return fmt.Errorf("python 3.12 does not have sub package %s", subpkg)
					}
				}
				return nil
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetEnv()
			execute(tc.queries, tc.regex, tc.listAllVersions, tc.showParentPkgInfo, tc.showSubPkgInfo, tc.outputJSON, "", "")

			if len(tc.errStreamContains) > 0 {
				b, err := io.ReadAll(ErrorStream)
				assert.NoError(t, err)
				for _, str := range tc.errStreamContains {
					assert.True(t, strings.Contains(string(b), str), "does not contain %s", str)
				}
			}

			output, err := io.ReadAll(WriteStream)
			assert.NoError(t, err)
			if len(tc.writeStreamContains) > 0 {
				for _, str := range tc.writeStreamContains {
					assert.True(t, strings.Contains(string(output), str), "does not contain %s", str)
				}
			}

			if tc.outputJSON {
				var pkgInfo map[string]PackageData
				err := json.Unmarshal(output, &pkgInfo)
				assert.NoError(t, err)
				err = tc.verifyJSON(pkgInfo)
				assert.NoError(t, err)
			}
		})
	}
}
