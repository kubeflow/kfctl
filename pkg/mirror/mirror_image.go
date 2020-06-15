package mirror

import (
	"fmt"
	"github.com/ghodss/yaml"
	mirrorv1alpha1 "github.com/kubeflow/kfctl/v3/pkg/apis/apps/imagemirror/v1alpha1"
	"github.com/kubeflow/kfctl/v3/pkg/kfapp/kustomize"
	"github.com/kubeflow/kfctl/v3/pkg/utils"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	pipeline "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	cloudbuild "google.golang.org/api/cloudbuild/v1"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const INPUT_IMAGE = "inputImage"
const OUTPUT_IMAGE = "outputImage"
const CONTEXT = "context"
const TASK_NAME = "mirror-image"
const KUSTOMIZE_FOLDER = "kustomize"
const CLOUD_BUILD_IMAGE = "gcr.io/cloud-builders/docker"
const CLOUD_BUILD_FILE = "cloudbuild.yaml"

type ReplicateTasks map[string]string

// buildContext: gs://<GCS bucket>/<path to .tar.gz>
func GenerateMirroringPipeline(directory string, spec mirrorv1alpha1.ReplicationSpec, outputFileName string, gcb bool) error {
	replicateTasks := make(ReplicateTasks)

	for _, pattern := range spec.Patterns {
		if err := replicateTasks.fillTasks(directory, pattern.Dest, spec.Context, pattern.Src.Include, pattern.Src.Exclude); err != nil {
			return err
		}
	}

	pipelineTasks := []pipeline.PipelineTask{}
	idx := 0
	re := regexp.MustCompile("[^a-z0-9]+")
	for _, newImg := range replicateTasks.orderedKeys() {
		oldImg := replicateTasks[newImg]
		pipelineTasks = append(pipelineTasks, pipeline.PipelineTask{
			Name: fmt.Sprintf("%v-%v", idx, re.ReplaceAllString(oldImg, "-")),
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
		return errors.WithStack(writeErr)
	}
	if gcb {
		return generateCloudBuild(replicateTasks)
	}
	return nil
}

func generateCloudBuild(rt ReplicateTasks) error {
	steps := []*cloudbuild.BuildStep{}
	images := []string{}
	for _, newImg := range rt.orderedKeys() {
		oldImg := rt[newImg]
		log.Infof("Add gcb step" + oldImg)
		steps = append(steps,
			&cloudbuild.BuildStep{
				Name:    CLOUD_BUILD_IMAGE,
				Args:    []string{"build", "-t", newImg, "--build-arg=INPUT_IMAGE=" + oldImg, "."},
				WaitFor: []string{"-"},
			},
		)
		images = append(images, newImg)
	}
	cb := cloudbuild.Build{
		Steps:  steps,
		Images: images,
	}
	buf, err := yaml.Marshal(cb)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(CLOUD_BUILD_FILE, buf, 0644)
}

func (rt *ReplicateTasks) orderedKeys() []string {
	var keys []string
	for k := range *rt {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// processKustomizeDir processes the specified kustomize directory
func (rt *ReplicateTasks) processKustomizeDir(absPath string, registry string, include string, exclude string) error {
	log.Infof("Processing %v", absPath)
	kustomizationFilePath := filepath.Join(absPath, "kustomization.yaml")
	if _, err := os.Stat(kustomizationFilePath); err != nil {
		log.Infof("Skipping %v; no kustomization.yaml found", absPath)
		return nil
	}
	kustomization := kustomize.GetKustomization(absPath)
	for _, image := range kustomization.Images {
		curName := image.Name
		if image.NewName != "" {
			curName = image.NewName
		}
		if strings.Contains(curName, "$") {
			log.Infof("Image name %v contains kutomize parameter, skipping\n", curName)
			continue
		}
		// check exclude first
		if exclude != "" && strings.HasPrefix(curName, exclude) {
			log.Infof("Image %v matches exclude prefix %v, skipping\n", curName, exclude)
			continue
		}
		// then check include
		if include != "" && (!strings.HasPrefix(curName, include)) {
			log.Infof("Image %v doesn't match include prefix %v, skipping\n", curName, include)
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

	// Process any kustomize packages we depend on.
	for _, r := range kustomization.Resources {
		if ext := strings.ToLower(filepath.Ext(r)); ext == ".yaml" || ext == ".yml" {
			continue
		}

		p := path.Join(absPath, r)

		if b, err := utils.IsRemoteFile(p); b || err != nil {
			if err != nil {
				log.Infof("Skipping path %v; there was an error determining if it was a local file; error: %v", p, err)
				continue
			}
			log.Infof("Skipping remote file %v", p)
			continue
		}
		if err := rt.processKustomizeDir(p, registry, include, exclude); err != nil {
			log.Errorf("Error occurred while processing %v; error %v", p, err)
		}
	}

	// Bases is deprecated but our manifests still use it.
	for _, r := range kustomization.Bases {
		p := path.Join(absPath, r)

		if b, err := utils.IsRemoteFile(p); b || err != nil {
			if err != nil {
				log.Infof("Skipping path %v; there was an error determining if it was a local file; error: %v", p, err)
				continue
			}
			log.Infof("Skipping remote file %v", p)
			continue
		}

		if err := rt.processKustomizeDir(p, registry, include, exclude); err != nil {
			log.Errorf("Error occurred while processing %v; error %v", p, err)
		}
	}
	return nil
}

func (rt *ReplicateTasks) fillTasks(directory string, registry string, buildContext string, include string, exclude string) error {
	return filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			absPath, err := filepath.Abs(path)
			if err != nil {
				return err
			}

			return rt.processKustomizeDir(absPath, registry, include, exclude)
		}

		return nil
	})
}

func verifyCurrDir() error {
	infos, err := ioutil.ReadDir(".")
	if err != nil {
		return errors.WithStack(err)
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
	if err := verifyCurrDir(); err != nil {
		return err
	}
	buf, err := ioutil.ReadFile(inputFile)
	if err != nil {
		return errors.WithStack(err)
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
		log.Infof("Updating image %v to %v", oldImg, newImg)
	}

	return filepath.Walk(KUSTOMIZE_FOLDER, func(path string, info os.FileInfo, err error) error {
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
						// drop image tag before write back to kustomize
						idx := strings.Index(newImg, ":")
						if idx == -1 {
							idx = strings.Index(newImg, "@")
						}
						kustomization.Images[i].NewName = newImg[:idx]
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
						return errors.WithStack(writeErr)
					}
				}
			}
			return nil
		}
		return nil
	})
}
