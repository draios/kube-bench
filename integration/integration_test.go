//go:build integration
// +build integration

package integration

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const KIND_CLUSTER_NAME = "integration-test"

var kubebenchImg = flag.String("kubebenchImg", "aquasec/kube-bench:latest", "kube-bench image used as part of this test")
var timeout = flag.Duration("timeout", 10*time.Minute, "Test Timeout")

func testCheckCISWithKind(t *testing.T, testdataDir string) {
	flag.Parse()
	fmt.Printf("kube-bench Container Image: %s\n", *kubebenchImg)

	cases := []struct {
		TestName      string
		KubebenchYAML string
		ExpectedFile  string
		ExpectError   bool
	}{
		{
			TestName:      "kube-bench",
			KubebenchYAML: "../job.yaml",
			ExpectedFile:  fmt.Sprintf("./testdata/%s/job.data", testdataDir),
		},
		{
			TestName:      "kube-bench-node",
			KubebenchYAML: "../job-node.yaml",
			ExpectedFile:  fmt.Sprintf("./testdata/%s/job-node.data", testdataDir),
		},
		{
			TestName:      "kube-bench-master",
			KubebenchYAML: "../job-master.yaml",
			ExpectedFile:  fmt.Sprintf("./testdata/%s/job-master.data", testdataDir),
		},
	}
	kubeDefaultPath := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	provider, err := setupCluster(KIND_CLUSTER_NAME, fmt.Sprintf("./testdata/%s/add-tls-kind.yaml", testdataDir), *timeout, kubeDefaultPath)
	if err != nil {
		t.Fatalf("failed to setup KIND cluster error: %v", err)
	}
	defer func() {
		provider.Delete(KIND_CLUSTER_NAME, kubeDefaultPath)
	}()

	if err := loadImageFromDocker(*kubebenchImg, provider, KIND_CLUSTER_NAME); err != nil {
		t.Fatalf("failed to load kube-bench image from Docker to KIND error: %v", err)
	}

	clientset, err := getClientSet(kubeDefaultPath)
	if err != nil {
		t.Fatalf("failed to connect to Kubernetes cluster error: %v", err)
	}

	for _, c := range cases {
		t.Run(c.TestName, func(t *testing.T) {
			resultData, err := runWithKind(clientset, c.TestName, c.KubebenchYAML, *kubebenchImg, *timeout)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			c, err := ioutil.ReadFile(c.ExpectedFile)
			if err != nil {
				t.Error(err)
			}

			expectedData := strings.TrimSpace(string(c))
			resultData = strings.TrimSpace(resultData)
			fmt.Print(resultData)
			if expectedData != resultData {
				t.Errorf("expected results\n\nExpected\t(<)\nResult\t(>)\n\n%s\n\n", generateDiff(expectedData, resultData))
			}
		})
	}
}

func TestCheckCIS15WithKind(t *testing.T) {
	testCheckCISWithKind(t, "cis-1.5")
}

func TestCheckCIS16WithKind(t *testing.T) {
	testCheckCISWithKind(t, "cis-1.6")
}

// This is simple "diff" between 2 strings containing multiple lines.
// It's not a comprehensive diff between the 2 strings.
// It does not inditcate when lines are deleted.
func generateDiff(source, target string) string {
	buf := new(bytes.Buffer)
	ss := bufio.NewScanner(strings.NewReader(source))
	ts := bufio.NewScanner(strings.NewReader(target))

	emptySource := false
	emptyTarget := false

loop:
	for ln := 1; ; ln++ {
		var ll, rl string

		sourceScan := ss.Scan()
		if sourceScan {
			ll = ss.Text()
		}

		targetScan := ts.Scan()
		if targetScan {
			rl = ts.Text()
		}

		switch {
		case !sourceScan && !targetScan:
			// no more lines
			break loop
		case sourceScan && targetScan:
			if ll != rl {
				fmt.Fprintf(buf, "line: %d\n", ln)
				fmt.Fprintf(buf, "< %s\n", ll)
				fmt.Fprintf(buf, "> %s\n", rl)
			}
		case !targetScan:
			if !emptyTarget {
				fmt.Fprintf(buf, "line: %d\n", ln)
			}
			fmt.Fprintf(buf, "< %s\n", ll)
			emptyTarget = true
		case !sourceScan:
			if !emptySource {
				fmt.Fprintf(buf, "line: %d\n", ln)
			}
			fmt.Fprintf(buf, "> %s\n", rl)
			emptySource = true
		}
	}

	if emptySource {
		fmt.Fprintf(buf, "< [[NO MORE DATA]]")
	}

	if emptyTarget {
		fmt.Fprintf(buf, "> [[NO MORE DATA]]")
	}

	return buf.String()
}
