package loaders

import (
	"fmt"

	"github.com/ghodss/yaml"
	kfapis "github.com/kubeflow/kfctl/v3/pkg/apis"
	kfdeftypes "github.com/kubeflow/kfctl/v3/pkg/apis/apps/kfdef/v1"
	kfdefgcpplugin "github.com/kubeflow/kfctl/v3/pkg/apis/apps/plugins/gcp/v1alpha1"
	"github.com/kubeflow/kfctl/v3/pkg/kfconfig"
)

// Empty struct - used to implement Converter interface.
type V1 struct {
}

func (v V1) LoadKfConfig(def interface{}) (*kfconfig.KfConfig, error) {
	kfdef := &kfdeftypes.KfDef{}
	if bytes, err := yaml.Marshal(def); err != nil {
		return nil, &kfapis.KfError{
			Code:    int(kfapis.INTERNAL_ERROR),
			Message: fmt.Sprintf("could not marshal kfdef into bytes: %v", err),
		}
	} else {
		err = yaml.Unmarshal(bytes, kfdef)
		if err != nil {
			return nil, &kfapis.KfError{
				Code:    int(kfapis.INTERNAL_ERROR),
				Message: fmt.Sprintf("could not unpack kfdef: %v", err),
			}
		}
	}

	// Set UseBasicAuth later.
	config := &kfconfig.KfConfig{
		Spec: kfconfig.KfConfigSpec{
			UseBasicAuth: false,
		},
	}
	config.Name = kfdef.Name
	config.Namespace = kfdef.Namespace
	config.APIVersion = kfdef.APIVersion
	config.Kind = "KfConfig"
	config.Labels = kfdef.Labels
	config.Annotations = kfdef.Annotations
	config.ClusterName = kfdef.ClusterName
	config.Spec.Version = kfdef.Spec.Version
	for i, app := range kfdef.Spec.Applications {
		if app.Name == "" {
			return nil, &kfapis.KfError{
				Code:    int(kfapis.INVALID_ARGUMENT),
				Message: fmt.Sprintf("must have name for application. missing application name on application[%d] in kfdef", i),
			}
		}
		application := kfconfig.Application{
			Name: app.Name,
		}
		if app.KustomizeConfig != nil {
			kconfig := &kfconfig.KustomizeConfig{
				Overlays: app.KustomizeConfig.Overlays,
			}
			if app.KustomizeConfig.RepoRef != nil {
				kref := &kfconfig.RepoRef{
					Name: app.KustomizeConfig.RepoRef.Name,
					Path: app.KustomizeConfig.RepoRef.Path,
				}
				kconfig.RepoRef = kref

				// Use application to infer whether UseBasicAuth is true.
				if kref.Path == "common/basic-auth" {
					config.Spec.UseBasicAuth = true
				}
			}
			for _, param := range app.KustomizeConfig.Parameters {
				p := kfconfig.NameValue{
					Name:  param.Name,
					Value: param.Value,
				}
				kconfig.Parameters = append(kconfig.Parameters, p)
			}
			application.KustomizeConfig = kconfig
		}
		config.Spec.Applications = append(config.Spec.Applications, application)
	}

	for _, plugin := range kfdef.Spec.Plugins {
		p := kfconfig.Plugin{
			Name:      plugin.Name,
			Namespace: kfdef.Namespace,
			Kind:      kfconfig.PluginKindType(plugin.Kind),
			Spec:      plugin.Spec,
		}
		config.Spec.Plugins = append(config.Spec.Plugins, p)

		if plugin.Kind == string(kfconfig.GCP_PLUGIN_KIND) {
			spec := kfdefgcpplugin.GcpPluginSpec{}
			if err := kfdef.GetPluginSpec(plugin.Kind, &spec); err != nil {
				return nil, &kfapis.KfError{
					Code:    int(kfapis.INTERNAL_ERROR),
					Message: fmt.Sprintf("could not retrieve GCP plugin spec: %v", err),
				}
			}

			config.Spec.Project = spec.Project
			config.Spec.Email = spec.Email
			config.Spec.IpName = spec.IpName
			config.Spec.Hostname = spec.Hostname
			config.Spec.SkipInitProject = spec.SkipInitProject
			config.Spec.Zone = spec.Zone
			config.Spec.DeleteStorage = spec.DeleteStorage
		}
		if p := maybeGetPlatform(plugin.Kind); p != "" {
			config.Spec.Platform = p
		}
	}

	for _, secret := range kfdef.Spec.Secrets {
		s := kfconfig.Secret{
			Name: secret.Name,
		}
		src := &kfconfig.SecretSource{}
		// kfdef -> kfconfig should keep  literalSource , becasue only kfdef should be checked into source control,
		// We only filter secrets during kfconfig -> kfdef.
		if secret.SecretSource.LiteralSource != nil {
			src.LiteralSource = &kfconfig.LiteralSource{
				Value: secret.SecretSource.LiteralSource.Value,
			}
		}
		if secret.SecretSource.EnvSource != nil {
			src.EnvSource = &kfconfig.EnvSource{
				Name: secret.SecretSource.EnvSource.Name,
			}
		}
		s.SecretSource = src
		config.Spec.Secrets = append(config.Spec.Secrets, s)
	}

	for _, repo := range kfdef.Spec.Repos {
		r := kfconfig.Repo{
			Name: repo.Name,
			URI:  repo.URI,
		}
		config.Spec.Repos = append(config.Spec.Repos, r)
	}

	for _, cond := range kfdef.Status.Conditions {
		c := kfconfig.Condition{
			Type:               kfconfig.ConditionType(cond.Type),
			Status:             cond.Status,
			LastUpdateTime:     cond.LastUpdateTime,
			LastTransitionTime: cond.LastTransitionTime,
			Reason:             cond.Reason,
			Message:            cond.Message,
		}
		config.Status.Conditions = append(config.Status.Conditions, c)
	}
	for _, cache := range kfdef.Status.ReposCache {
		c := kfconfig.Cache{
			Name:      cache.Name,
			LocalPath: cache.LocalPath,
		}
		config.Status.Caches = append(config.Status.Caches, c)
	}

	return config, nil
}

func (v V1) LoadKfDef(config kfconfig.KfConfig, out interface{}) error {
	kfdef := &kfdeftypes.KfDef{}
	kfdef.Name = config.Name
	kfdef.Namespace = config.Namespace
	kfdef.APIVersion = config.APIVersion
	kfdef.Kind = "KfDef"
	kfdef.Labels = config.Labels
	kfdef.Annotations = config.Annotations
	kfdef.ClusterName = config.ClusterName
	kfdef.Spec.Version = config.Spec.Version

	for _, app := range config.Spec.Applications {
		application := kfdeftypes.Application{
			Name: app.Name,
		}
		if app.KustomizeConfig != nil {
			kconfig := &kfdeftypes.KustomizeConfig{
				Overlays: app.KustomizeConfig.Overlays,
			}
			if app.KustomizeConfig.RepoRef != nil {
				kref := &kfdeftypes.RepoRef{
					Name: app.KustomizeConfig.RepoRef.Name,
					Path: app.KustomizeConfig.RepoRef.Path,
				}
				kconfig.RepoRef = kref
			}
			for _, param := range app.KustomizeConfig.Parameters {
				p := kfdeftypes.NameValue{
					Name:  param.Name,
					Value: param.Value,
				}
				kconfig.Parameters = append(kconfig.Parameters, p)
			}
			application.KustomizeConfig = kconfig
		}
		kfdef.Spec.Applications = append(kfdef.Spec.Applications, application)
	}

	for _, plugin := range config.Spec.Plugins {
		p := kfdeftypes.Plugin{
			Spec: plugin.Spec,
		}
		p.Name = plugin.Name
		p.Kind = string(plugin.Kind)
		kfdef.Spec.Plugins = append(kfdef.Spec.Plugins, p)
	}

	for _, secret := range config.Spec.Secrets {
		s := kfdeftypes.Secret{
			Name: secret.Name,
		}
		if secret.SecretSource != nil {
			s.SecretSource = &kfdeftypes.SecretSource{}
			// We don't want to store literalSource explictly, becasue we want the config to be checked into source control and don't want secrets in source control.
			if secret.SecretSource.EnvSource != nil {
				s.SecretSource.EnvSource = &kfdeftypes.EnvSource{
					Name: secret.SecretSource.EnvSource.Name,
				}
			}
		}
		kfdef.Spec.Secrets = append(kfdef.Spec.Secrets, s)
	}

	for _, repo := range config.Spec.Repos {
		r := kfdeftypes.Repo{
			Name: repo.Name,
			URI:  repo.URI,
		}
		kfdef.Spec.Repos = append(kfdef.Spec.Repos, r)
	}

	for _, cond := range config.Status.Conditions {
		c := kfdeftypes.KfDefCondition{
			Type:               kfdeftypes.KfDefConditionType(cond.Type),
			Status:             cond.Status,
			LastUpdateTime:     cond.LastUpdateTime,
			LastTransitionTime: cond.LastTransitionTime,
			Reason:             cond.Reason,
			Message:            cond.Message,
		}
		kfdef.Status.Conditions = append(kfdef.Status.Conditions, c)
	}

	for _, cache := range config.Status.Caches {
		c := kfdeftypes.RepoCache{
			Name:      cache.Name,
			LocalPath: cache.LocalPath,
		}
		kfdef.Status.ReposCache = append(kfdef.Status.ReposCache, c)
	}

	kfdefBytes, err := yaml.Marshal(kfdef)
	if err != nil {
		return &kfapis.KfError{
			Code:    int(kfapis.INTERNAL_ERROR),
			Message: fmt.Sprintf("error when marshaling to KfDef: %v", err),
		}
	}

	err = yaml.Unmarshal(kfdefBytes, out)
	if err == nil {
		return nil
	} else {
		return &kfapis.KfError{
			Code:    int(kfapis.INTERNAL_ERROR),
			Message: fmt.Sprintf("error when unmarshaling to KfDef: %v", err),
		}
	}
}
