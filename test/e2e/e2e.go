// Copyright 2019 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

/*
Copyright 2019 The Kubernetes Authors.

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

// Modified from https://github.com/kubernetes/kubernetes/blob/v1.13.0-beta.0/test/e2e/e2e.go

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	"github.com/onsi/ginkgo/reporters"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeutils "k8s.io/apimachinery/pkg/util/runtime"
	utilversion "k8s.io/apimachinery/pkg/util/version"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/component-base/logs"
	"k8s.io/klog"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
	imageutils "k8s.io/kubernetes/test/utils/image"
)

const (
	asNamespace = "advanced-statefulset"
)

var (
	images = []string{
		imageutils.GetE2EImage(imageutils.Httpd),
		imageutils.GetE2EImage(imageutils.HttpdNew),
		imageutils.GetE2EImage(imageutils.Redis),
		imageutils.GetE2EImage(imageutils.Kitten),
		imageutils.GetE2EImage(imageutils.Nautilus),
	}
)

var _ = ginkgo.SynchronizedBeforeSuite(func() []byte {
	framework.SetupSuite()
	// Load images
	kindPath := filepath.Join(framework.TestContext.RepoRoot, "output/bin/linux/kind")
	for _, image := range images {
		e2elog.Logf("Loading image %s", image)
		if err := exec.Command("docker", "pull", image).Run(); err != nil {
			framework.ExpectNoError(err)
		}
		if err := exec.Command(kindPath, "load", "docker-image", "--name", "advanced-statefulset", image).Run(); err != nil {
			framework.ExpectNoError(err)
		}
	}
	// Get the client
	config, err := framework.LoadConfig()
	c, err := clientset.NewForConfig(config)
	framework.ExpectNoError(err, "failed to create clientset")
	// Install CRDs
	gte116, err := framework.ServerVersionGTE(utilversion.MustParseSemantic("v1.16.0"), c.Discovery())
	framework.ExpectNoError(err)
	if gte116 {
		framework.RunKubectlOrDie("apply", "-f", filepath.Join(framework.TestContext.RepoRoot, "deployment/crd.v1.yaml"))
	} else {
		framework.RunKubectlOrDie("apply", "-f", filepath.Join(framework.TestContext.RepoRoot, "deployment/crd.v1beta1.yaml"))
	}
	framework.RunKubectlOrDie("wait", "--for=condition=Established", "crds/statefulsets.apps.pingcap.com")
	// Install Controller
	_, err = c.CoreV1().Namespaces().Create(&v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: asNamespace,
		},
	})
	framework.ExpectNoError(err, "failed to create namespace")
	framework.RunKubectlOrDie("apply", "-f", filepath.Join(framework.TestContext.RepoRoot, "deployment/rbac.yaml"))
	framework.RunKubectlOrDie("apply", "-f", filepath.Join(framework.TestContext.RepoRoot, "deployment/deployment.yaml"))
	framework.RunKubectlOrDie("-n", asNamespace, "wait", "--for=condition=Available", "deployment/advanced-statefulset-controller")
	return nil
}, func(data []byte) {
	// Run on all Ginkgo nodes
	framework.SetupSuitePerGinkgoNode()
})

var _ = ginkgo.SynchronizedAfterSuite(func() {
	framework.CleanupSuite()
}, func() {
	framework.AfterSuiteActions()
})

// RunE2ETests checks configuration parameters (specified through flags) and then runs
// E2E tests using the Ginkgo runner.
// If a "report directory" is specified, one or more JUnit test reports will be
// generated in this directory, and cluster logs will also be saved.
// This function is called on each Ginkgo node in parallel mode.
func RunE2ETests(t *testing.T) {
	runtimeutils.ReallyCrash = true
	logs.InitLogs()
	defer logs.FlushLogs()

	gomega.RegisterFailHandler(e2elog.Fail)

	// Disable skipped tests unless they are explicitly requested.
	if config.GinkgoConfig.FocusString == "" && config.GinkgoConfig.SkipString == "" {
		config.GinkgoConfig.SkipString = `\[Flaky\]|\[Feature:.+\]`
	}

	// Run tests through the Ginkgo runner with output to console + JUnit for Jenkins
	var r []ginkgo.Reporter
	if framework.TestContext.ReportDir != "" {
		// TODO: we should probably only be trying to create this directory once
		// rather than once-per-Ginkgo-node.
		if err := os.MkdirAll(framework.TestContext.ReportDir, 0755); err != nil {
			klog.Errorf("Failed creating report directory: %v", err)
		} else {
			r = append(r, reporters.NewJUnitReporter(path.Join(framework.TestContext.ReportDir, fmt.Sprintf("junit_%v%02d.xml", framework.TestContext.ReportPrefix, config.GinkgoConfig.ParallelNode))))
		}
	}
	klog.Infof("Starting e2e run %q on Ginkgo node %d", framework.RunID, config.GinkgoConfig.ParallelNode)

	ginkgo.RunSpecsWithDefaultAndCustomReporters(t, "Kubernetes e2e suite", r)
}
