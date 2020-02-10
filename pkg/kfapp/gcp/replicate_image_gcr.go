package gcp

import (
	"github.com/kubeflow/kfctl/v3/pkg/kfapp/kustomize"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	log "github.com/sirupsen/logrus"
	"github.com/ghodss/yaml"
	"time"
)

type ReplicateTask struct {
	oldImg string
	newImg string
}

func ReplicateToGcr(registry string, include string, exclude string) error {
	replicateTasks := []ReplicateTask{}
	// used to tag images specified by digest
	defaultTag := "autotag-v" + time.Now().Format("20060102150405")
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
							newImg: strings.Join([]string{image.NewName, image.NewTag}, ":"),
						})
					}
					if image.Digest != "" {
						replicateTasks = append(replicateTasks, ReplicateTask{
							oldImg: strings.Join([]string{image.Name, image.Digest}, "@"),
							newImg: strings.Join([]string{image.NewName, defaultTag}, ":"),
						})
					}

					log.Infof("Replacing image name from %s to %s", image.Name, newName)
					kustomization.Images[i].NewName = newName
				}

				data, err := yaml.Marshal(kustomization)
				if err != nil {
					return err
				}

				writeErr := ioutil.WriteFile(kustomizationFilePath, data, 0644)
				if writeErr != nil {
					return writeErr
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
	return nil

}
