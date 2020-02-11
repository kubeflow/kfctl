package gcp

import (
	"fmt"
	"github.com/ghodss/yaml"
	"github.com/kubeflow/kfctl/v3/pkg/kfapp/kustomize"
	log "github.com/sirupsen/logrus"
	pipeline "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"path/filepath"
	"strings"
)

type ReplicateTask struct {
	oldImg string
	newImg string
}

// buildContext: gs://<GCS bucket>/<path to .tar.gz>
func GenerateReplicationPipeline(registry string, buildContext string, include string, exclude string) error {
	replicateTasks := []ReplicateTask{}
	// used to tag images specified by digest
	//defaultTag := "autotag-v" + time.Now().Format("20060102150405")
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			absPath, err := filepath.Abs(path)
			if err != nil {
				return err
			}

			kustomizationFilePath := filepath.Join(absPath, "kustomization.yaml")
			if _, err := os.Stat(kustomizationFilePath); err == nil {
				kustomization := kustomize.GetKustomization(absPath)
				for i, image := range kustomization.Images {
					curName := image.Name
					if image.NewName != "" {
						curName = image.NewName
					}
					// check exclude first
					if strings.HasPrefix(curName, exclude) {
						log.Infof("image %v matches exclude prefix %v, skipping\n", curName, exclude)
						continue
					}
					// then check include
					if include != "" && (!strings.HasPrefix(curName, include)) {
						log.Infof("image %v doesn't match include prefix %v, skipping\n", curName, include)
						continue
					}
					newName := strings.Join([]string{registry, image.Name}, "/")

					if (image.NewTag == "") == (image.Digest == "") {
						log.Warnf("One and only one of NewTag or Digest can exist for image %s, skipping\n",
							image.Name)
						continue
					}

					if image.NewTag != "" {
						replicateTasks = append(replicateTasks, ReplicateTask{
							oldImg: strings.Join([]string{image.Name, image.NewTag}, ":"),
							newImg: strings.Join([]string{newName, image.NewTag}, ":"),
						})
					}
					if image.Digest != "" {
						replicateTasks = append(replicateTasks, ReplicateTask{
							oldImg: strings.Join([]string{image.Name, image.Digest}, "@"),
							newImg: strings.Join([]string{newName, image.Digest}, "@"),
						})
					}

					log.Infof("Replacing image name from %s to %s", image.Name, newName)
					kustomization.Images[i].NewName = newName
				}
			}
			return nil
		}

		return nil
	})
	if err != nil {
		return err
	}
	// TODO: create image replicate pipeline
	//data, err := yaml.Marshal(replicateTasks)

	taskIns := pipeline.Task{
		TypeMeta: v1.TypeMeta{
			APIVersion: "tekton.dev/v1alpha1",
			Kind:       "Task",
		},
		ObjectMeta: v1.ObjectMeta{
			Name: "images-replication",
		},
		Spec: pipeline.TaskSpec{
			Inputs: &pipeline.Inputs{
				Params: []pipeline.ParamSpec{
					{
						Name: "inputImage",
						Type: pipeline.ParamTypeString,
					},
					{
						Name: "outputImage",
						Type: pipeline.ParamTypeString,
					},
				},
			},
			Steps: []pipeline.Step{
				{
					Container: corev1.Container{
						Name:  "build-push",
						Image: "gcr.io/kaniko-project/executor:latest",
						Args: []string{
							"--dockerfile=.",
							"--context=" + buildContext,
							"--destination=$(OUTPUT_IMAGE)",
							"--build-arg INPUT_IMAGE=$(INPUT_IMAGE)",
						},
						Env: []corev1.EnvVar{
							{
								Name:  "INPUT_IMAGE",
								Value: "$(inputs.params.inputImage)",
							},
							{
								Name:  "OUTPUT_IMAGE",
								Value: "$(inputs.params.outputImage)",
							},
						},
					},
				},
			},
		},
	}

	pipelineTasks := []pipeline.PipelineTask{}
	for idx, re := range replicateTasks {
		pipelineTasks = append(pipelineTasks, pipeline.PipelineTask{
			Name: fmt.Sprintf("replicate-%v", idx),
			TaskRef: &pipeline.TaskRef{
				Name: "images-replication",
			},
			Params: []pipeline.Param{
				{
					Name: "inputImage",
					Value: pipeline.ArrayOrString{
						Type:      pipeline.ParamTypeString,
						StringVal: re.oldImg,
					},
				},
				{
					Name: "outputImage",
					Value: pipeline.ArrayOrString{
						Type:      pipeline.ParamTypeString,
						StringVal: re.newImg,
					},
				},
			},
		})
	}
	pipelineIns := pipeline.Pipeline{
		TypeMeta: v1.TypeMeta{
			APIVersion: "tekton.dev/v1alpha1",
			Kind:       "Pipeline",
		},
		ObjectMeta: v1.ObjectMeta{
			Name: "images-replication",
		},
		Spec: pipeline.PipelineSpec{
			Tasks: pipelineTasks,
		},
	}
	buf, err := yaml.Marshal(taskIns)
	if err != nil {
		return err
	}
	buf = append(buf, []byte("\n---\n")...)
	buf2, err := yaml.Marshal(pipelineIns)
	if err != nil {
		return err
	}
	buf = append(buf, buf2...)
	writeErr := ioutil.WriteFile("replicate.yaml", buf, 0644)
	if writeErr != nil {
		return writeErr
	}
	return nil
}
