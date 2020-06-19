package mirror

import (
	"bytes"
	mirrorv1alpha1 "github.com/kubeflow/kfctl/v3/pkg/apis/apps/imagemirror/v1alpha1"
	"github.com/kubeflow/kfctl/v3/pkg/utils"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
	"path"
	"testing"
)

const KUSTOMIZATION = "kustomize/kustomization.yaml"

type testCase struct {
	Expected string
	Actual   string
}

func init() {
	if err := os.Chdir("testdata"); err != nil {
		log.Errorf("Failed to change dir %v", err)
	}
}

func TestGenerateMirroringPipeline(t *testing.T) {
	spec := mirrorv1alpha1.ReplicationSpec{
		Patterns: []mirrorv1alpha1.Pattern{
			{
				Src: mirrorv1alpha1.SrcImages{
					Exclude: "gcr.io",
				},
				Dest: "gcr.io/kubeflow-dev",
			},
		},
		Context: "gs://kubeflow-examples/image-replicate/replicate-context.tar.gz",
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Could not get working directory; error %v", err)
	}
	directory := path.Join(wd, "kustomize")
	if err := GenerateMirroringPipeline(directory, spec, "pipeline.yaml", true); err != nil {
		t.Error(err)
	}
	defer func(t *testing.T) {
		if err := os.Remove("pipeline.yaml"); err != nil {
			t.Error(err)
		}
		if err := os.Remove("cloudbuild.yaml"); err != nil {
			t.Error(err)
		}
	}(t)
	cases := []testCase{
		{
			Expected: "expected-pipeline.yaml",
			Actual:   "pipeline.yaml",
		},
		{
			Expected: "expected-cloudbuild.yaml",
			Actual:   "cloudbuild.yaml",
		},
	}
	compFile(cases, t)
}

func TestUpdateKustomize(t *testing.T) {

	original, err := ioutil.ReadFile(KUSTOMIZATION)
	if err != nil {
		t.Error(err)
	}
	// Undo change after test run
	defer func(t *testing.T) {
		if err := ioutil.WriteFile(KUSTOMIZATION, original, 0644); err != nil {
			t.Error(err)
		}
	}(t)
	if err := UpdateKustomize("expected-pipeline.yaml"); err != nil {
		t.Error(err)
	}
	cases := []testCase{
		{
			Expected: "expected-kustomization.yaml",
			Actual:   KUSTOMIZATION,
		},
	}
	compFile(cases, t)
}

func compFile(cases []testCase, t *testing.T) {
	for _, ca := range cases {
		expectedBytes, err := ioutil.ReadFile(ca.Expected)
		if err != nil {
			t.Error(err)
		}
		actualBytes, err := ioutil.ReadFile(ca.Actual)
		if err != nil {
			t.Error(err)
		}
		if !bytes.Equal(expectedBytes, actualBytes) {
			utils.PrintDiff(string(actualBytes), string(expectedBytes))
			t.Errorf("Result not matching; got\n%v\nwant\n%v", string(actualBytes), string(expectedBytes))
		}
	}
}
