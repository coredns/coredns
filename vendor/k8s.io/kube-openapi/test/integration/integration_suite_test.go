/*
Copyright 2018 The Kubernetes Authors.

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

package integration

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

const (
	testdataDir          = "./testdata"
	inputDir             = testdataDir + "/listtype" + "," + testdataDir + "/dummytype"
	outputBase           = "pkg"
	outputPackage        = "generated"
	outputBaseFilename   = "openapi_generated"
	generatedSwaggerFile = "generated.json"
	generatedReportFile  = "generated.report"
	goldenSwaggerFile    = "golden.json"
	goldenReportFile     = "golden.report"
	timeoutSeconds       = 5.0
)

func TestGenerators(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Test Suite")
}

var _ = Describe("Open API Definitions Generation", func() {

	var (
		tempDir             string
		terr                error
		generatedSwaggerDef string
		generatedReportDef  string
	)

	BeforeSuite(func() {
		// Create a temporary directory for generated swagger files.
		tempDir, terr = ioutil.TempDir("", "openapi")
		Expect(terr).ShouldNot(HaveOccurred())
		// Build the OpenAPI code generator.
		binary_path, berr := gexec.Build("../../cmd/openapi-gen/openapi-gen.go")
		Expect(berr).ShouldNot(HaveOccurred())
		generatedReportDef = filepath.Join(tempDir, generatedReportFile)
		// Run the OpenAPI code generator, creating OpenAPIDefinition code
		// to be compiled into builder.
		command := exec.Command(binary_path, "-i", inputDir, "-o", outputBase, "-p", outputPackage, "-O", outputBaseFilename, "-r", generatedReportDef)
		session, err := gexec.Start(command, GinkgoWriter, GinkgoWriter)
		Expect(err).ShouldNot(HaveOccurred())
		Eventually(session, timeoutSeconds).Should(gexec.Exit(0))

		// Create the OpenAPI swagger builder.
		binary_path, berr = gexec.Build("./builder/main.go")
		Expect(berr).ShouldNot(HaveOccurred())
		// Execute the builder, generating an OpenAPI swagger file with definitions.
		generatedSwaggerDef = filepath.Join(tempDir, generatedSwaggerFile)
		command = exec.Command(binary_path, generatedSwaggerDef)
		command.Dir = testdataDir
		session, err = gexec.Start(command, GinkgoWriter, GinkgoWriter)
		Expect(err).ShouldNot(HaveOccurred())
		Eventually(session, timeoutSeconds).Should(gexec.Exit(0))
	})

	AfterSuite(func() {
		os.RemoveAll(tempDir)
		gexec.CleanupBuildArtifacts()
	})

	Describe("Validating OpenAPI Definition Generation", func() {
		It("Generated OpenAPI swagger definitions should match golden files", func() {
			// Diff the generated swagger against the golden swagger. Exit code should be zero.
			command := exec.Command("diff", goldenSwaggerFile, generatedSwaggerDef)
			command.Dir = testdataDir
			session, err := gexec.Start(command, GinkgoWriter, GinkgoWriter)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(session, timeoutSeconds).Should(gexec.Exit(0))
		})
	})

	Describe("Validating API Rule Violation Reporting", func() {
		It("Generated API rule violations should match golden report files", func() {
			// Diff the generated report against the golden report. Exit code should be zero.
			command := exec.Command("diff", goldenReportFile, generatedReportDef)
			command.Dir = testdataDir
			session, err := gexec.Start(command, GinkgoWriter, GinkgoWriter)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(session, timeoutSeconds).Should(gexec.Exit(0))
		})
	})
})
