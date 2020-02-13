package gcp

import (
	"fmt"
	"github.com/ghodss/yaml"
	utilsv1alpha1 "github.com/kubeflow/kfctl/v3/pkg/apis/apps/utils/v1alpha1"
	"github.com/kubeflow/kfctl/v3/pkg/kfapp/kustomize"
	log "github.com/sirupsen/logrus"
	pipeline "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"path/filepath"
	"strings"
)

const INPUT_IMAGE = "inputImage"
const OUTPUT_IMAGE = "outputImage"
const CONTEXT = "context"
const TASK_NAME = "images-replication"
const KUSTOMIZE_FOLDER = "kustomize"

type ReplicateTasks map[string]string

// buildContext: gs://<GCS bucket>/<path to .tar.gz>
func GenerateReplicationPipeline(spec utilsv1alpha1.ReplicationSpec,
	//
	outputFileName string) error {
	replicateTasks := make(ReplicateTasks)
	if err := verifyCurrDir(); err != nil {
		return err
	}

	for _, pattern := range spec.Patterns {
		if err := replicateTasks.fillTasks(pattern.Dest, spec.Context, pattern.Src.Include, pattern.Src.Exclude); err != nil {
			return err
		}
	}

	pipelineTasks := []pipeline.PipelineTask{}
	idx := 0
	for newImg, oldImg := range replicateTasks {
		pipelineTasks = append(pipelineTasks, pipeline.PipelineTask{
			Name: fmt.Sprintf("replicate-%v", idx),
			TaskRef: &pipeline.TaskRef{
				Name: TASK_NAME,
			},
			Params: []pipeline.Param{
				{
					Name: INPUT_IMAGE,
					Value: pipeline.ArrayOrString{
						Type:      pipeline.ParamTypeString,
						StringVal: oldImg,
					},
				},
				{
					Name: OUTPUT_IMAGE,
					Value: pipeline.ArrayOrString{
						Type:      pipeline.ParamTypeString,
						StringVal: newImg,
					},
				},
				{
					Name: CONTEXT,
					Value: pipeline.ArrayOrString{
						Type:      pipeline.ParamTypeString,
						StringVal: "$(params.context)",
					},
				},
			},
		})
		idx++
	}
	pipelineIns := pipeline.PipelineRun{
		TypeMeta: v1.TypeMeta{
			APIVersion: "tekton.dev/v1alpha1",
			Kind:       "PipelineRun",
		},
		ObjectMeta: v1.ObjectMeta{
			Name: "replication-pipeline",
		},
		Spec: pipeline.PipelineRunSpec{
			PipelineSpec: &pipeline.PipelineSpec{
				Tasks: pipelineTasks,
				Params: []pipeline.ParamSpec{
					{
						Name: CONTEXT,
						Type: pipeline.ParamTypeString,
					},
				},
			},
			Params: []pipeline.Param{
				{
					Name: CONTEXT,
					Value: pipeline.ArrayOrString{
						Type:      pipeline.ParamTypeString,
						StringVal: spec.Context,
					},
				},
			},
		},
	}
	buf, err := yaml.Marshal(pipelineIns)
	if err != nil {
		return err
	}
	writeErr := ioutil.WriteFile(outputFileName, buf, 0644)
	if writeErr != nil {
		return writeErr
	}
	return nil
}

func (rt *ReplicateTasks) fillTasks(registry string, buildContext string, include string, exclude string) error {
	return filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			absPath, err := filepath.Abs(path)
			if err != nil {
				return err
			}

			kustomizationFilePath := filepath.Join(absPath, "kustomization.yaml")
			if _, err := os.Stat(kustomizationFilePath); err == nil {
				kustomization := kustomize.GetKustomization(absPath)
				for _, image := range kustomization.Images {
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
						(*rt)[strings.Join([]string{newName, image.NewTag}, ":")] =
							strings.Join([]string{curName, image.NewTag}, ":")
					}
					if image.Digest != "" {
						(*rt)[strings.Join([]string{newName, image.Digest}, "@")] =
							strings.Join([]string{curName, image.Digest}, "@")
					}
					log.Infof("Replacing image name from %s to %s", image.Name, newName)
					//kustomization.Images[i].NewName = newName
				}
			}
			return nil
		}

		return nil
	})
}

func verifyCurrDir() error {
	infos, err := ioutil.ReadDir(".")
	if err != nil {
		return err
	}
	foundKus := false
	for _, info := range infos {
		if info.IsDir() && info.Name() == KUSTOMIZE_FOLDER {
			foundKus = true
			break
		}
	}
	if !foundKus {
		return fmt.Errorf("kustomize folder not found, have you executed kfctl build yet?")
	}
	return nil
}

func UpdateKustomize(inputFile string) error {
	buf, err := ioutil.ReadFile(inputFile)
	if err != nil {
		return err
	}
	pipelineRun := pipeline.PipelineRun{}
	if err := yaml.Unmarshal(buf, &pipelineRun); err != nil {
		return err
	}
	imageMapping := make(map[string]string)
	for _, task := range pipelineRun.Spec.PipelineSpec.Tasks {
		oldImg := ""
		newImg := ""
		for _, param := range task.Params {
			if param.Name == INPUT_IMAGE {
				oldImg = param.Value.StringVal
			}
			if param.Name == OUTPUT_IMAGE {
				newImg = param.Value.StringVal
			}
		}
		imageMapping[oldImg] = newImg
		log.Infof("updating image %v to %v", oldImg, newImg)
	}

	return filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			absPath, err := filepath.Abs(path)
			if err != nil {
				return err
			}

			kustomizationFilePath := filepath.Join(absPath, "kustomization.yaml")
			if _, err := os.Stat(kustomizationFilePath); err == nil {
				kustomization := kustomize.GetKustomization(absPath)
				rewrite := false
				for i, image := range kustomization.Images {
					curName := image.Name
					if image.NewName != "" {
						curName = image.NewName
					}
					if (image.NewTag == "") == (image.Digest == "") {
						log.Warnf("One and only one of NewTag or Digest can exist for image %s, skipping\n",
							image.Name)
						continue
					}
					if image.NewTag != "" {
						curName = strings.Join([]string{curName, image.NewTag}, ":")
					}
					if image.Digest != "" {
						curName = strings.Join([]string{curName, image.Digest}, "@")
					}
					if newImg, ok := imageMapping[curName]; ok {
						kustomization.Images[i].NewName = newImg
						rewrite = true
					}
				}
				if rewrite {
					data, err := yaml.Marshal(kustomization)
					if err != nil {
						return err
					}

					writeErr := ioutil.WriteFile(kustomizationFilePath, data, 0644)
					if writeErr != nil {
						return writeErr
					}
				}
			}
			return nil
		}
		return nil
	})
}
