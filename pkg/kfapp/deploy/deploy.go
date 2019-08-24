package deploy

import (
	"errors"
	"os"

	kftypes "github.com/kubeflow/kfctl/v3/pkg/apis/apps"
	kfdefsv3 "github.com/kubeflow/kfctl/v3/pkg/apis/apps/kfdef/v1alpha1"
	"github.com/kubeflow/kfctl/v3/pkg/kfapp/coordinator"
	log "github.com/sirupsen/logrus"
)

// InstallKubeflow installs Kubeflow using the coordinator package
func InstallKubeflow(appName string, configFile string) error {
	// Create a kf-app config with the app name from CLI and internal config
	kfDef := &kfdefsv3.KfDef{}
	kfDef, err := kfdefsv3.LoadKFDefFromURI(configFile)
	if err != nil {
		log.Printf("Unable to create KfDef from config file: %v", err)
	}
	if kfDef.Name != "" {
		log.Warnf("Overriding KfDef.Spec.Name; old value %v; new value %v", kfDef.Name, appName)
	}
	kfDef.Name = appName
	isValid, msg := kfDef.IsValid()
	if !isValid {
		log.Printf("Invalid kfdef: %v", isValid)
		log.Printf("Error validating generated KfDef, please check config file validity: %v", msg)
	}
	kfDef.Spec.AppDir = CreateAppDir(appName)
	if kfDef.Spec.AppDir == "" {
		return errors.New("kfDef App Dir not set")
	}
	log.Warnf("App directory name: %v", kfDef.Spec.AppDir)
	cfgFilePath, err := coordinator.CreateKfAppCfgFile(kfDef)
	if err != nil {
		return err
	}

	log.Printf("Syncing Cache")
	err = kfDef.SyncCache()
	if err != nil {
		log.Errorf("Failed to synchronize the cache; error: %v", err)
		return err
	}
	// Save app.yaml because we need to preserve information about the cache.
	if err := kfDef.WriteToFile(cfgFilePath); err != nil {
		log.Errorf("Failed to save KfDef to %v; error %v", cfgFilePath, err)
		return err
	}
	log.Printf("Saved configfile as kfdef in path: %v", cfgFilePath)

	// Load KfApp for Generate and Apply
	KfApp, KfErr := coordinator.LoadKfAppCfgFile(cfgFilePath)
	if KfErr != nil {
		log.Printf("Error loading KfApp from configfilepath: %v", KfErr)
	}
	// Once init is done, we generate and apply subsequently
	kfResource := kftypes.K8S
	log.Println("Kubeflow Generate...")
	generateErr := KfApp.Generate(kfResource)
	if generateErr != nil {
		log.Println("Unable to generate resources for KfApp", generateErr)
		return generateErr
	}
	log.Println("Kubeflow Apply...")
	applyErr := KfApp.Apply(kfResource)
	if applyErr != nil {
		log.Println("Unable to apply resources for KfApp", applyErr)
	}
	return nil
}

// CreateAppDir creates a project directory for installing components
func CreateAppDir(appName string) string {
	cwd, err := os.Getwd()
	if err != nil {
		log.Printf("Error getting current working directory: %v", err)
	}
	appdirErr := os.MkdirAll(cwd+"/"+appName, os.ModePerm)
	if appdirErr != nil {
		log.Errorf("couldn't create directory %v Error %v", appName, appdirErr)
		return ""
	}
	return cwd + "/" + appName
}
