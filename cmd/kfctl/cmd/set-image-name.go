package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	kftypes "github.com/kubeflow/kfctl/v3/pkg/apis/apps"
	"github.com/kubeflow/kfctl/v3/pkg/kfapp/kustomize"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

var filter string
var flatten bool

func init() {
	setImageNameCmd.Flags().StringVarP(&filter, "filter", "f", "", "Only set name for images with matching prefix")
	setImageNameCmd.Flags().BoolVar(&flatten, "flatten", false, "Set to true for registries not supporting hierarchical paths with more than two components")
	setImageNameCmd.Flags().BoolP(string(kftypes.VERBOSE), "V", false, "Enable verbose output")
	rootCmd.AddCommand(setImageNameCmd)

}

var setImageNameCmd = &cobra.Command{
	Use:   "set-image-name <prefix>",
	Short: "Custom image names for kubeflow components",
	Long: `Sets custom image names for kubeflow components.

Replaces the image name in kubeflow manifests with the specified prefix, to support custom image registries.
It assumes that all components specify images in kustomization.yaml, base or overlay. Expected prefix format is
<registry>[:port][/component]*

The filter flag sets the custom image name only for images with matching prefix.
The flatten flag discards both registry and name components except for the last one, to support registries with a flat hierarchical path.
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		log.SetLevel(log.InfoLevel)
		log.Debugf("Using prefix %s (filter %s, flatten %t)\n", args[0], filter, flatten)

		newNameComponents, err := parsePrefix(args[0])
		if err != nil {
			return err
		}

		return filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				log.Debugf("looking for kustomization.yaml in %q\n", path)
				absPath, err := filepath.Abs(path)
				if err != nil {
					return err
				}

				kustomizationFilePath := filepath.Join(absPath, "kustomization.yaml")
				if _, err := os.Stat(kustomizationFilePath); err == nil {
					kustomization := kustomize.GetKustomization(absPath)
					for i, image := range kustomization.Images {
						if !strings.HasPrefix(image.Name, filter) {
							log.Infof("No filter match for %s, skipping\n", image.Name)
							continue
						}

						newName := setImageName(image.Name, newNameComponents)
						log.Infof("Replacing image name from %s to %s", image.Name, newName)
						kustomization.Images[i].NewName = newName
					}

					if err := os.Remove(kustomizationFilePath); err != nil {
						return err
					}

					data, err := yaml.Marshal(kustomization)
					if err != nil {
						return err
					}

					file, err := os.OpenFile(kustomizationFilePath, os.O_WRONLY|os.O_CREATE, 0644)
					if err != nil {
						return err
					}
					defer file.Close()

					if _, err := file.Write(data); err != nil {
						return err
					}
				}

				return nil
			}

			return nil
		})
	},
}

func parsePrefix(prefix string) (map[string]string, error) {
	prefixParser := regexp.MustCompile(`^(?P<host>(\w[\w-0-9]+\.)+\w+)(:(?P<port>\d+))?(/(?P<components>([\w-_/]*)))?$`)
	match := prefixParser.FindStringSubmatch(prefix)
	if len(match) == 0 {
		return nil, fmt.Errorf("Unsupported prefix %s", prefix)
	}

	result := make(map[string]string)

	for i, name := range prefixParser.SubexpNames() {
		if i != 0 && name != "" {
			result[name] = match[i]
		}
	}

	return result, nil
}

func parseImageName(name string) map[string]string {
	imageParser := regexp.MustCompile(`^((?P<host>(\w[\w-0-9]+\.)+\w+)(:(?P<port>\d+))?/)?((?P<components>([\w-_]+/)*([\w-_]+))/)?(?P<image>[\w-_]*)(:(?P<tag>.*))?$`)
	match := imageParser.FindStringSubmatch(name)

	if len(match) == 0 {
		return nil
	}

	result := make(map[string]string)

	for i, name := range imageParser.SubexpNames() {
		if i != 0 && name != "" {
			result[name] = match[i]
		}
	}

	return result
}

func setImageName(oldName string, newNameComponents map[string]string) string {
	oldNameComponents := parseImageName(oldName)

	domainSlice := []string{newNameComponents["host"]}
	if newNameComponents["port"] != "" {
		domainSlice = append(domainSlice, newNameComponents["port"])
	}
	domain := strings.Join(domainSlice, ":")

	imageSlice := []string{oldNameComponents["image"]}
	if oldNameComponents["tag"] != "" {
		imageSlice = append(imageSlice, oldNameComponents["tag"])
	}
	image := strings.Join(imageSlice, ":")

	resultSlice := []string{domain}
	if newNameComponents["components"] != "" {
		resultSlice = append(resultSlice, newNameComponents["components"])
	}

	if !flatten {
		resultSlice = append(resultSlice, oldNameComponents["components"])
	}

	resultSlice = append(resultSlice, image)

	return strings.Join(resultSlice, "/")
}
