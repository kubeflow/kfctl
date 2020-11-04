package kfconfig

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"github.com/ghodss/yaml"
	"github.com/hashicorp/go-getter/helper/url"
	kfapis "github.com/kubeflow/kfctl/v3/pkg/apis"
	kftypesv3 "github.com/kubeflow/kfctl/v3/pkg/apis/apps"
	"github.com/otiai10/copy"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sigs.k8s.io/kustomize/v3/pkg/types"
	"strings"
)

const (
	DefaultCacheDir = ".cache"
	// KfAppsStackName is the name that should be assigned to the application corresponding to the kubeflow
	// application stack.
	KfAppsStackName = "kubeflow-apps"
	KustomizeDir    = "kustomize"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Internal data structure to hold app related info.
// +k8s:openapi-gen=true
type KfConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KfConfigSpec `json:"spec,omitempty"`
	Status Status       `json:"status,omitempty"`
}

// The spec of kKfConfig
type KfConfigSpec struct {
	// Shared fields among all components. should limit this list.
	// TODO(gabrielwen): Deprecate AppDir and move it to cache in Status.
	AppDir string `json:"appDir,omitempty"`
	// The filename of the config, e.g. app.yaml.
	// Base name only, as the directory is AppDir above.
	ConfigFileName string `json:"configFileName,omitempty"`

	Version string `json:"version,omitempty"`

	// TODO(gabrielwen): Can we infer this from Applications?
	UseBasicAuth bool `json:"useBasicAuth,omitempty"`

	Platform string `json:"platform,omitempty"`

	// TODO(gabrielwen): Deprecate these fields as they only makes sense to GCP.
	Project         string `json:"project,omitempty"`
	Email           string `json:"email,omitempty"`
	IpName          string `json:"ipName,omitempty"`
	Hostname        string `json:"hostname,omitempty"`
	SkipInitProject bool   `json:"skipInitProject,omitempty"`
	Zone            string `json:"zone,omitempty"`

	DeleteStorage bool `json:"deleteStorage,omitempty"`

	Applications []Application `json:"applications,omitempty"`
	Plugins      []Plugin      `json:"plugins,omitempty"`
	Secrets      []Secret      `json:"secrets,omitempty"`
	Repos        []Repo        `json:"repos,omitempty"`
}

// Application defines an application to install
type Application struct {
	Name            string           `json:"name,omitempty"`
	KustomizeConfig *KustomizeConfig `json:"kustomizeConfig,omitempty"`
}

type KustomizeConfig struct {
	RepoRef    *RepoRef    `json:"repoRef,omitempty"`
	Overlays   []string    `json:"overlays,omitempty"`
	Parameters []NameValue `json:"parameters,omitempty"`
}

type RepoRef struct {
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
}

type NameValue struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

type Plugin struct {
	Name      string                `json:"name,omitempty"`
	Namespace string                `json:"namespace,omitempty"`
	Kind      PluginKindType        `json:"kind,omitempty"`
	Spec      *runtime.RawExtension `json:"spec,omitempty"`
}

// Secret provides information about secrets needed to configure Kubeflow.
// Secrets can be provided via references.
type Secret struct {
	Name         string        `json:"name,omitempty"`
	SecretSource *SecretSource `json:"secretSource,omitempty"`
}

type SecretSource struct {
	LiteralSource *LiteralSource `json:"literalSource,omitempty"`
	HashedSource  *HashedSource  `json:"hashedSource,omitempty"`
	EnvSource     *EnvSource     `json:"envSource,omitempty"`
}

type LiteralSource struct {
	Value string `json:"value,omitempty"`
}

type HashedSource struct {
	HashedValue string `json:"value,omitempty"`
}

type EnvSource struct {
	Name string `json:"name,omitempty"`
}

// SecretRef is a reference to a secret
type SecretRef struct {
	// Name of the secret
	Name string `json:"name,omitempty"`
}

// Repo provides information about a repository providing config (e.g. kustomize packages,
// Deployment manager configs, etc...)
type Repo struct {
	// Name is a name to identify the repository.
	Name string `json:"name,omitempty"`
	// URI where repository can be obtained.
	// Can use any URI understood by go-getter:
	// https://github.com/hashicorp/go-getter/blob/master/README.md#installation-and-usage
	URI string `json:"uri,omitempty"`
}

type Status struct {
	Conditions []Condition `json:"conditions,omitempty"`
	Caches     []Cache     `json:"caches,omitempty"`
}

type Condition struct {
	// Type of deployment condition.
	Type ConditionType `json:"type,omitempty"`
	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status,omitempty"`
	// The last time this condition was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}

type Cache struct {
	Name      string `json:"name,omitempty"`
	LocalPath string `json:"localPath,omitempty"`
}

type PluginKindType string

const (
	// Used for populating plugin missing errors and identifying those
	// errors.
	pluginNotFoundErrPrefix = "Missing plugin"

	// Used for populating plugin missing errors and identifying those
	// errors.
	conditionNotFoundErrPrefix = "Missing condition"
)

// Plugin kind used starting from v1beta1
const (
	AWS_PLUGIN_KIND              PluginKindType = "KfAwsPlugin"
	GCP_PLUGIN_KIND              PluginKindType = "KfGcpPlugin"
	MINIKUBE_PLUGIN_KIND         PluginKindType = "KfMinikubePlugin"
	EXISTING_ARRIKTO_PLUGIN_KIND PluginKindType = "KfExistingArriktoPlugin"
)

type ConditionType string

const (
	// Available means Kubeflow is serving.
	Available ConditionType = "Available"

	// Degraded means one or more Kubeflow services are not healthy.
	Degraded ConditionType = "Degraded"

	// Pending means Kubeflow services is being updated.
	Pending ConditionType = "Pending"
)

// Define plugin related conditions to be the format:
// - conditions for successful plugins: ${PluginKind}Succeeded
// - conditions for failed plugins: ${PluginKind}Failed
func GetPluginSucceededCondition(pluginKind PluginKindType) ConditionType {
	return ConditionType(fmt.Sprintf("%vSucceeded", pluginKind))
}
func GetPluginFailedCondition(pluginKind PluginKindType) ConditionType {
	return ConditionType(fmt.Sprintf("%vFailed", pluginKind))
}

// Returns the repo with the name and true if repo exists.
// nil and false otherwise.
func (c *KfConfig) GetRepoCache(repoName string) (Cache, bool) {
	for _, r := range c.Status.Caches {
		if r.Name == repoName {
			return r, true
		}
	}
	return Cache{}, false
}

func (c *KfConfig) GetPluginSpec(pluginKind PluginKindType, s interface{}) error {
	for _, p := range c.Spec.Plugins {
		if p.Kind != pluginKind {
			continue
		}

		// To deserialize it to a specific type we need to first serialize it to bytes
		// and then unserialize it.
		specBytes, err := yaml.Marshal(p.Spec)
		if err != nil {
			msg := fmt.Sprintf("Could not marshal plugin %v args; error %v", pluginKind, err)
			log.Errorf(msg)
			return &kfapis.KfError{
				Code:    int(kfapis.INVALID_ARGUMENT),
				Message: msg,
			}
		}
		err = yaml.Unmarshal(specBytes, s)
		if err != nil {
			msg := fmt.Sprintf("Could not unmarshal plugin %v to the provided type; error %v", pluginKind, err)
			log.Errorf(msg)
			return &kfapis.KfError{
				Code:    int(kfapis.INTERNAL_ERROR),
				Message: msg,
			}
		}
		return nil
	}
	return &kfapis.KfError{
		Code:    int(kfapis.NOT_FOUND),
		Message: fmt.Sprintf("%v %v", pluginNotFoundErrPrefix, pluginKind),
	}
}

// SetPluginSpec sets the requested parameter: add the plugin if it doesn't already exist, or replace existing plugin.
func (c *KfConfig) SetPluginSpec(pluginKind PluginKindType, spec interface{}) error {
	// Convert spec to RawExtension
	r := &runtime.RawExtension{}

	// To deserialize it to a specific type we need to first serialize it to bytes
	// and then unserialize it.
	specBytes, err := yaml.Marshal(spec)

	if err != nil {
		msg := fmt.Sprintf("Could not marshal spec; error %v", err)
		log.Errorf(msg)
		return &kfapis.KfError{
			Code:    int(kfapis.INVALID_ARGUMENT),
			Message: msg,
		}
	}

	err = yaml.Unmarshal(specBytes, r)

	if err != nil {
		msg := fmt.Sprintf("Could not unmarshal plugin to RawExtension; error %v", err)
		log.Errorf(msg)
		return &kfapis.KfError{
			Code:    int(kfapis.INTERNAL_ERROR),
			Message: msg,
		}
	}

	index := -1

	for i, p := range c.Spec.Plugins {
		if p.Kind == pluginKind {
			index = i
			break
		}
	}

	if index == -1 {
		// Plugin in doesn't exist so add it
		log.Infof("Adding plugin %v", pluginKind)

		p := Plugin{}
		p.Name = string(pluginKind)
		p.Kind = pluginKind
		c.Spec.Plugins = append(c.Spec.Plugins, p)

		index = len(c.Spec.Plugins) - 1
	}

	c.Spec.Plugins[index].Spec = r
	return nil
}

// Sets condition and status to KfConfig.
func (c *KfConfig) SetCondition(condType ConditionType,
	status v1.ConditionStatus,
	reason string,
	message string) {
	now := metav1.Now()
	cond := Condition{
		Type:               condType,
		Status:             status,
		LastUpdateTime:     now,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}

	for i := range c.Status.Conditions {
		if c.Status.Conditions[i].Type != condType {
			continue
		}
		if c.Status.Conditions[i].Status == status {
			cond.LastTransitionTime = c.Status.Conditions[i].LastTransitionTime
		}
		c.Status.Conditions[i] = cond
		return
	}
	c.Status.Conditions = append(c.Status.Conditions, cond)
}

// Gets condition from KfConfig.
func (c *KfConfig) GetCondition(condType ConditionType) (*Condition, error) {
	for i := range c.Status.Conditions {
		if c.Status.Conditions[i].Type == condType {
			return &c.Status.Conditions[i], nil
		}
	}
	return nil, &kfapis.KfError{
		Code:    int(kfapis.NOT_FOUND),
		Message: fmt.Sprintf("%v %v", conditionNotFoundErrPrefix, condType),
	}
}

func (c *KfConfig) IsPluginFinished(pluginKind PluginKindType) bool {
	condType := GetPluginSucceededCondition(pluginKind)
	cond, err := c.GetCondition(condType)
	if err != nil {
		if IsConditionNotFound(err) {
			return false
		}
		log.Warnf("Error when getting condition info: %v", err)
		return false
	}
	return cond.Status == v1.ConditionTrue
}

func (c *KfConfig) SetPluginFinished(pluginKind PluginKindType, msg string) {
	succeededCond := GetPluginSucceededCondition(pluginKind)
	failedCond := GetPluginFailedCondition(pluginKind)
	if _, err := c.GetCondition(failedCond); err == nil {
		c.SetCondition(failedCond, v1.ConditionFalse, "",
			"Reset to false as the plugin is set to be finished.")
	}

	c.SetCondition(succeededCond, v1.ConditionTrue, "", msg)
}

func (c *KfConfig) IsPluginFailed(pluginKind PluginKindType) bool {
	condType := GetPluginFailedCondition(pluginKind)
	cond, err := c.GetCondition(condType)
	if err != nil {
		if IsConditionNotFound(err) {
			return false
		}
		log.Warnf("Error when getting condition info: %v", err)
		return false
	}
	return cond.Status == v1.ConditionTrue
}

func (c *KfConfig) SetPluginFailed(pluginKind PluginKindType, msg string) {
	succeededCond := GetPluginSucceededCondition(pluginKind)
	failedCond := GetPluginFailedCondition(pluginKind)
	if _, err := c.GetCondition(succeededCond); err == nil {
		c.SetCondition(succeededCond, v1.ConditionFalse,
			"", "Reset to false as the plugin is set to be failed.")
	}

	c.SetCondition(failedCond, v1.ConditionTrue, "", msg)
}

// SyncCache will synchronize the local cache of any repositories.
// On success the status is updated with pointers to the cache.
//
// TODO(jlewi): I'm not sure this handles head references correctly.
// e.g. suppose we have a URI like
// https://github.com/kubeflow/manifests/tarball/pull/189/head?archive=tar.gz
// This gets unpacked to: kubeflow-manifests-e2c1bcb where e2c1bcb is the commit.
// I don't think the code is currently setting the local directory for the cache correctly in
// that case.
//
//
// Using tarball vs. archive in github links affects the download path
// e.g.
// https://github.com/kubeflow/manifests/tarball/master?archive=tar.gz
//    unpacks to  kubeflow-manifests-${COMMIT}
// https://github.com/kubeflow/manifests/archive/master.tar.gz
//    unpacks to manifests-master
// Always use archive format so that the path is predetermined.
//
// Instructions: https://github.com/hashicorp/go-getter#protocol-specific-options
//
// What is the correct syntax for downloading pull requests?
// The following doesn't seem to work
// https://github.com/kubeflow/manifests/archive/master.tar.gz?ref=pull/188
//   * Appears to download master
//
// This appears to work
// https://github.com/kubeflow/manifests/tarball/pull/188/head?archive=tar.gz
// But unpacks it into
// kubeflow-manifests-${COMMIT}
//
func (c *KfConfig) SyncCache() error {
	if c.Spec.AppDir == "" {
		return fmt.Errorf("AppDir must be specified")
	}

	appDir := c.Spec.AppDir
	// Loop over all the repos and download them.
	// TODO(https://github.com/kubeflow/kubeflow/issues/3545): We should check if we already have a local copy and
	// not redownload it.

	baseCacheDir := path.Join(appDir, DefaultCacheDir)
	if _, err := os.Stat(baseCacheDir); os.IsNotExist(err) {
		log.Infof("Creating directory %v", baseCacheDir)
		appdirErr := os.MkdirAll(baseCacheDir, os.ModePerm)
		if appdirErr != nil {
			log.Errorf("Couldn't create directory %v: %v", baseCacheDir, appdirErr)
			return appdirErr
		}
	}

	for _, r := range c.Spec.Repos {
		cacheDir := path.Join(baseCacheDir, r.Name)

		// Can we use a checksum or other mechanism to verify if the existing location is good?
		// If there was a problem the first time around then removing it might provide a way to recover.
		if _, err := os.Stat(cacheDir); err == nil {
			// Check if the cache is up to date.
			shouldSkip := false
			for _, cache := range c.Status.Caches {
				if cache.Name == r.Name && cache.LocalPath != "" {
					shouldSkip = true
					break
				}
			}
			if shouldSkip {
				log.Infof("%v exists; not resyncing ", cacheDir)
				continue
			}

			log.Infof("Deleting cachedir %v because Status.ReposCache is out of date", cacheDir)

			// TODO(jlewi): The reason the cachedir might exist but not be stored in KfDef.status
			// is because of a backwards compatibility path in which we download the cache to construct
			// the KfDef. Specifically coordinator.CreateKfDefFromOptions is calling kftypes.DownloadFromCache
			// We don't want to rely on that method to set the cache because we have logic
			// below to set LocalPath that we don't want to duplicate.
			// Unfortunately this means we end up fetching the repo twice which is very inefficient.
			if err := os.RemoveAll(cacheDir); err != nil {
				log.Errorf("There was a problem deleting directory %v; error %v", cacheDir, err)
				return errors.WithStack(err)
			}
		}

		u, err := url.Parse(r.URI)

		if err != nil {
			log.Errorf("Could not parse URI %v; error %v", r.URI, err)
			return errors.WithStack(err)
		}

		log.Infof("Fetching %v to %v", r.URI, cacheDir)
		if err := os.MkdirAll(cacheDir, os.ModePerm); err != nil {
			log.Errorf("Could not create dir %v; error %v", cacheDir, err)
			return errors.WithStack(err)
		}

		// Manifests are local dir
		if fi, err := os.Stat(r.URI); err == nil && fi.Mode().IsDir() {
			// check whether the cache directory is a sub directory of manifests
			absCacheDir, err := filepath.Abs(cacheDir)
			if err != nil {
				return errors.WithStack(err)
			}

			absURI, err := filepath.Abs(r.URI)
			if err != nil {
				return errors.WithStack(err)
			}

			relDir, err := filepath.Rel(absURI, absCacheDir)
			if err != nil {
				return errors.WithStack(err)
			}

			if !strings.HasPrefix(relDir, ".."+string(filepath.Separator)) {
				return errors.WithStack(errors.New("SyncCache: could not sync cache when the cache path " + cacheDir + " is sub directory of manifests " + r.URI))
			}

			if err := copy.Copy(r.URI, cacheDir); err != nil {
				return errors.WithStack(err)
			}
		} else {
			t := &http.Transport{
				Proxy: http.ProxyFromEnvironment,
			}
			t.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))
			t.RegisterProtocol("", http.NewFileTransport(http.Dir("/")))
			hclient := &http.Client{Transport: t}
			req, _ := http.NewRequest("GET", r.URI, nil)
			req.Header.Set("User-Agent", "kfctl")
			resp, err := hclient.Do(req)
			if err != nil {
				return &kfapis.KfError{
					Code:    int(kfapis.INVALID_ARGUMENT),
					Message: fmt.Sprintf("couldn't download URI %v: %v", r.URI, err),
				}
			}
			defer resp.Body.Close()

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Errorf("Could not read response body; error %v", err)
				return errors.WithStack(err)
			}
			if err := untar(body, cacheDir); err != nil {
				log.Errorf("Could not untar file %v; error %v", r.URI, err)
				return errors.WithStack(err)
			}
		}

		// This is a bit of a hack to deal with the fact that GitHub tarballs
		// can unpack to a directory containing the commit.
		localPath := cacheDir
		files, filesErr := ioutil.ReadDir(cacheDir)
		if filesErr != nil {
			log.Errorf("Error reading cachedir; error %v", filesErr)
			return errors.WithStack(filesErr)
		}
		if u.Scheme == "http" || u.Scheme == "https" {
			subdir := files[0].Name()
			localPath = path.Join(cacheDir, subdir)
			log.Infof("Updating localPath to %v", localPath)
		} else if u.Scheme == "file" {
			filePath := strings.TrimPrefix(r.URI, "file:")
			log.Infof("Probing file path: %v", filePath)
			if fileInfo, err := os.Stat(filePath); err != nil {
				return &kfapis.KfError{
					Code:    int(kfapis.INVALID_ARGUMENT),
					Message: fmt.Sprintf("couldn't stat the path %v: %v", filePath, err),
				}
			} else if !fileInfo.IsDir() {
				subdir := files[0].Name()
				localPath = path.Join(cacheDir, subdir)
				log.Infof("Updating localPath to %v", localPath)
			}
		}

		c.Status.Caches = append(c.Status.Caches, Cache{
			Name:      r.Name,
			LocalPath: localPath,
		})

		log.Infof("Fetch succeeded; LocalPath %v", localPath)
	}
	return nil
}

func untar(body []byte, cacheDir string) error {
	gzf, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return err
	}

	tarReader := tar.NewReader(gzf)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header == nil {
			continue
		}

		target := filepath.Join(cacheDir, header.Name)

		switch header.Typeflag {

		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}

		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			if _, err := io.Copy(f, tarReader); err != nil {
				return err
			}

			if err := f.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}

// GetSecret returns the specified secret or an error if the secret isn't specified.
func (c *KfConfig) GetSecret(name string) (string, error) {
	for _, s := range c.Spec.Secrets {
		if s.Name != name {
			continue
		}
		if s.SecretSource.LiteralSource != nil {
			return s.SecretSource.LiteralSource.Value, nil
		}
		if s.SecretSource.HashedSource != nil {
			return s.SecretSource.HashedSource.HashedValue, nil
		}
		if s.SecretSource.EnvSource != nil {
			return os.Getenv(s.SecretSource.EnvSource.Name), nil
		}

		return "", fmt.Errorf("No secret source provided for secret %v", name)
	}
	return "", NewSecretNotFound(name)
}

// GetSecretSource returns the SecretSource of the specified name or an error if the secret isn't specified.
func (c *KfConfig) GetSecretSource(name string) (*SecretSource, error) {
	for _, s := range c.Spec.Secrets {
		if s.Name == name {
			return s.SecretSource, nil
		}
	}
	return nil, NewSecretNotFound(name)
}

// GetApplicationParameter gets the desired application parameter.
func (c *KfConfig) GetApplicationParameter(appName string, paramName string) (string, bool) {
	// First we check applications for an application with the specified name.
	if c.Spec.Applications != nil {
		for _, a := range c.Spec.Applications {
			if a.Name == appName {
				return getParameter(a.KustomizeConfig.Parameters, paramName)
			}
		}
	}

	return "", false
}

// addPatchStratgicMerge adds the patchFile to the strategic merge if it isn't already present.
// Returns true if it is added
func addPatchStratgicMerge(k *types.Kustomization, patchFile string) bool {
	for _, p := range k.PatchesStrategicMerge {
		if string(p) == patchFile {
			log.Infof("kustomization already defines a patch for %v", patchFile)
			return false
		}
	}

	k.PatchesStrategicMerge = append(k.PatchesStrategicMerge, types.PatchStrategicMerge(patchFile))

	return true
}

// setApplicationParameterInConfigMap sets an application parameter by creatign or modifying a configMap
// generator.
// kustomizeDir: Directory of the kustomize application
// appName: Name of the application
// paramName: Name of the parameter
// value: Value of the parameter
//
// N.B. In the YAML for the generated config map patch the creationTimeStamp is set to null. This appears to
// be the result of how the struct is serialized. Hopefully having this field in the output doesn't cause problems
// with kustomize and kubectl.
func setApplicationParameterInConfigMap(kustomizeDir string, appName string, paramName string, value string) error {
	if _, err := os.Stat(kustomizeDir); err == nil {
		// Noop if the directory already exists.
	} else if os.IsNotExist(err) {
		log.Infof("Creating kustomize directory %v", kustomizeDir)
		if err := os.MkdirAll(kustomizeDir, os.ModePerm); err != nil {
			return errors.WithStack(errors.Wrapf(err, "Could not create directory: %v", kustomizeDir))
		}
	} else {
		log.Errorf("Error checking directory %v; error %v", kustomizeDir, err)
		return errors.WithStack(err)
	}

	kustomizationFile := filepath.Join(kustomizeDir, kftypesv3.KustomizationFile)

	contents, err := ioutil.ReadFile(kustomizationFile)

	k := &types.Kustomization{}

	// The kustomization file may not exist yet in which case we keep going because we will just create it.
	if err == nil {
		if err := yaml.Unmarshal(contents, k); err != nil {
			return errors.WithStack(errors.Wrapf(err, "Failed to unmashal kustomization.yaml: %v", kustomizationFile))
		}
	} else if err != nil && !os.IsNotExist(err) {
		return errors.WithStack(errors.Wrapf(err, "Failed to read: %v", kustomizationFile))
	}

	configMapFileName := appName + "-config.yaml"

	if addPatchStratgicMerge(k, configMapFileName) {
		yaml, err := yaml.Marshal(k)

		if err != nil {
			return errors.WithStack(errors.Wrapf(err, "Error trying to marshal kustomization for kubeflow application: %v", appName))
		}

		kustomizationFileErr := ioutil.WriteFile(kustomizationFile, yaml, 0644)
		if kustomizationFileErr != nil {
			return errors.WithStack(errors.Wrapf(kustomizationFileErr, "Error writing file: %v", kustomizationFile))
		}
	}

	// Patch the parameter into the configmap.
	c := &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              appName + "-config",
			CreationTimestamp: metav1.Time{},
		},
	}

	configMapPath := filepath.Join(kustomizeDir, configMapFileName)
	if contents, err := ioutil.ReadFile(configMapPath); err == nil {
		if err := yaml.Unmarshal(contents, c); err != nil {
			return errors.WithStack(errors.Wrapf(err, "Error reading configmap from file: %v", configMapPath))
		}
	} else if !os.IsNotExist(err) {
		return errors.WithStack(errors.Wrapf(err, "Error trying to read file: %v", configMapPath))
	}

	if c.Data == nil {
		c.Data = map[string]string{}
	}
	c.Data[paramName] = value

	newContents, err := yaml.Marshal(c)

	if err != nil {
		return errors.WithStack(errors.Wrapf(err, "Error while marshaling patch for configMap %v", configMapPath))
	}

	if err := ioutil.WriteFile(configMapPath, newContents, os.ModePerm); err != nil {
		return errors.WithStack(errors.Wrapf(err, "Error while writing patch file: %v", configMapPath))
	}

	return nil
}

// legacySetApplicationParameter sets the desired application parameter.
//
// This is the legacy version of KFDef that predates the use of kustomize stacks. In this case
// application parameters are set by modifying the Applications in the KFDef spec.
func (c *KfConfig) legacySetApplicationParameter(appName string, paramName string, value string) error {
	// First we check applications for an application with the specified name.
	if c.Spec.Applications != nil {
		appIndex := -1
		for i, a := range c.Spec.Applications {
			if a.Name == appName {
				appIndex = i
			}
		}

		if appIndex >= 0 {

			if c.Spec.Applications[appIndex].KustomizeConfig == nil {
				return errors.WithStack(fmt.Errorf("Application %v doesn't have KustomizeConfig", appName))
			}

			c.Spec.Applications[appIndex].KustomizeConfig.Parameters = setParameter(
				c.Spec.Applications[appIndex].KustomizeConfig.Parameters, paramName, value)

			return nil
		}

	}
	log.Warnf("Application %v not found", appName)
	return nil
}

// SetApplicationParameter sets the desired application parameter.
func (c *KfConfig) SetApplicationParameter(appName string, paramName string, value string) error {
	if c.UsingStacks() {
		// We need to map the application names to the stack they belong to.
		// Prior to the v3 version which introduced the stack there was a 1:1 mapping between the appName
		// and the kustomize directory for that application.
		// With the introduction of stacks some of the applications e.g. "jupyter-web-app" are now in the
		// the kubeflow-apps stack. So when we call SetApplicationParameter("jupyter-web-app",...)
		// we actually want to modify the config map inside ${KFAPP}/kustomize/kubeflow-apps
		//
		// TODO(jlewi): Is there a better way handle this other than hardcoding the path.
		appToStack := map[string]string{
			"centraldashboard": KfAppsStackName,
			"cloud-endpoints":  "cloud-endpoints",
			"default-install":  KfAppsStackName,
			"istio-stack":      "istio-stack",
			"iap-ingress":      "iap-ingress",
			"jupyter-web-app":  KfAppsStackName,
			"metacontroller":   "metacontroller",
			"profiles":         KfAppsStackName,
			"dex":              "dex",
			// Spartakus is its own application because we want kfctl to be able to remove it.
			"spartakus":                  "spartakus",
			// AWS Specific
			"aws-alb-ingress-controller": KfAppsStackName,
			"istio-ingress":              "istio-ingress",
		}

		appNameDir, ok := appToStack[appName]

		if !ok {
			// Default to assuming appNameDir is the same as appName if not explicitly
			// specified?
			appNameDir = appName
			log.Warnf("No stack directory specified for app %v; defaulting to %v", appName, appNameDir)
		}
		kustomizeDir := filepath.Join(c.Spec.AppDir, KustomizeDir, appNameDir)
		return setApplicationParameterInConfigMap(kustomizeDir, appName, paramName, value)
	}
	return c.legacySetApplicationParameter(appName, paramName, value)
}

// UsingStacks returns true if the KfDef is using kustomize to collect all of the Kubeflow applications
func (c *KfConfig) UsingStacks() bool {
	if c.Spec.Applications == nil {
		return false
	}

	for _, a := range c.Spec.Applications {
		if a.Name == KfAppsStackName {
			return true
		}
	}
	return false
}
func (c *KfConfig) DeleteApplication(appName string) error {
	// First we check applications for an application with the specified name.
	if c.Spec.Applications != nil {
		appIndex := -1
		for i, a := range c.Spec.Applications {
			if a.Name == appName {
				appIndex = i
			}
		}

		if appIndex >= 0 {
			c.Spec.Applications = append(c.Spec.Applications[:appIndex], c.Spec.Applications[appIndex+1:]...)
			return nil
		}

	}
	log.Warnf("Application %v not found", appName)
	return nil
}

func (c *KfConfig) AddApplicationOverlay(appName, overlayName string) error {
	// First we check applications for an application with the specified name.
	if c.Spec.Applications != nil {
		appIndex := -1
		for i, a := range c.Spec.Applications {
			if a.Name == appName {
				appIndex = i
			}
		}

		if appIndex >= 0 {
			overlayIndex := -1
			for i, o := range c.Spec.Applications[appIndex].KustomizeConfig.Overlays {
				if o == overlayName {
					overlayIndex = i
				}
			}

			if overlayIndex >= 0 {
				log.Warnf("Found existing overlay %v in Application %v, skip adding", appName, overlayName)
			} else {
				c.Spec.Applications[appIndex].KustomizeConfig.Overlays = append(c.Spec.Applications[appIndex].KustomizeConfig.Overlays, overlayName)
			}
		} else {
			log.Warnf("Application %v not found, overlay %v cannot be added", appName, overlayName)
		}
	}

	return nil
}

func (c *KfConfig) RemoveApplicationOverlay(appName, overlayName string) error {
	// First we check applications for an application with the specified name.
	if c.Spec.Applications != nil {
		appIndex := -1
		for i, a := range c.Spec.Applications {
			if a.Name == appName {
				appIndex = i
			}
		}

		if appIndex >= 0 {
			overlayIndex := -1
			for i, o := range c.Spec.Applications[appIndex].KustomizeConfig.Overlays {
				if o == overlayName {
					overlayIndex = i
				}
			}

			if overlayIndex >= 0 {
				c.Spec.Applications[appIndex].KustomizeConfig.Overlays = append(c.Spec.Applications[appIndex].KustomizeConfig.Overlays[:overlayIndex],
					c.Spec.Applications[appIndex].KustomizeConfig.Overlays[overlayIndex+1:]...)
			} else {
				log.Warnf("Cannot find overlay %v in Application %v, skip removing", appName, overlayName)
			}
		} else {
			log.Warnf("Application %v not found, overlay %v cannot be deleted", appName, overlayName)
		}
	}

	return nil
}

// SetSecret sets the specified secret; if a secret with the given name already exists it is overwritten.
func (c *KfConfig) SetSecret(newSecret Secret) {
	for i, s := range c.Spec.Secrets {
		if s.Name == newSecret.Name {
			c.Spec.Secrets[i] = newSecret
			return
		}
	}

	c.Spec.Secrets = append(c.Spec.Secrets, newSecret)
}

func IsPluginNotFound(e error) bool {
	if e == nil {
		return false
	}
	err, ok := e.(*kfapis.KfError)
	return ok && err.Code == int(kfapis.NOT_FOUND) && strings.HasPrefix(err.Message, pluginNotFoundErrPrefix)
}

func IsConditionNotFound(e error) bool {
	if e == nil {
		return false
	}
	err, ok := e.(*kfapis.KfError)
	return ok && err.Code == int(kfapis.NOT_FOUND) &&
		strings.HasPrefix(err.Message, conditionNotFoundErrPrefix)
}

type SecretNotFound struct {
	Name string
}

func (e *SecretNotFound) Error() string {
	return fmt.Sprintf("Missing secret %v", e.Name)
}

func NewSecretNotFound(n string) *SecretNotFound {
	return &SecretNotFound{
		Name: n,
	}
}

func IsSecretNotFound(e error) bool {
	if e == nil {
		return false
	}
	_, ok := e.(*SecretNotFound)
	return ok
}

type AppNotFound struct {
	Name string
}

func (e *AppNotFound) Error() string {
	return fmt.Sprintf("Application %v is missing", e.Name)
}

func IsAppNotFound(e error) bool {
	if e == nil {
		return false
	}
	_, ok := e.(*AppNotFound)
	return ok
}

func getParameter(parameters []NameValue, paramName string) (string, bool) {
	for _, p := range parameters {
		if p.Name == paramName {
			return p.Value, true
		}
	}

	return "", false
}

func setParameter(parameters []NameValue, paramName string, value string) []NameValue {
	pIndex := -1

	for i, p := range parameters {
		if p.Name == paramName {
			pIndex = i
		}
	}

	if pIndex < 0 {
		parameters = append(parameters, NameValue{})
		pIndex = len(parameters) - 1
	}

	parameters[pIndex].Name = paramName
	parameters[pIndex].Value = value

	return parameters
}
