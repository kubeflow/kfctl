// +build !ignore_autogenerated

/*
Copyright The Kubernetes Authors.

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

// Code generated by deepcopy-gen. DO NOT EDIT.

package v1alpha1

import (
	v1beta1 "github.com/kubeflow/kfctl/v3/pkg/apis/apps/kfdef/v1beta1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Auth) DeepCopyInto(out *Auth) {
	*out = *in
	if in.BasicAuth != nil {
		in, out := &in.BasicAuth, &out.BasicAuth
		*out = new(BasicAuth)
		(*in).DeepCopyInto(*out)
	}
	if in.IAP != nil {
		in, out := &in.IAP, &out.IAP
		*out = new(IAP)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Auth.
func (in *Auth) DeepCopy() *Auth {
	if in == nil {
		return nil
	}
	out := new(Auth)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BasicAuth) DeepCopyInto(out *BasicAuth) {
	*out = *in
	if in.Password != nil {
		in, out := &in.Password, &out.Password
		*out = new(v1beta1.SecretRef)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BasicAuth.
func (in *BasicAuth) DeepCopy() *BasicAuth {
	if in == nil {
		return nil
	}
	out := new(BasicAuth)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DeploymentManagerConfig) DeepCopyInto(out *DeploymentManagerConfig) {
	*out = *in
	if in.RepoRef != nil {
		in, out := &in.RepoRef, &out.RepoRef
		*out = new(v1beta1.RepoRef)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DeploymentManagerConfig.
func (in *DeploymentManagerConfig) DeepCopy() *DeploymentManagerConfig {
	if in == nil {
		return nil
	}
	out := new(DeploymentManagerConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GcpPluginSpec) DeepCopyInto(out *GcpPluginSpec) {
	*out = *in
	if in.Auth != nil {
		in, out := &in.Auth, &out.Auth
		*out = new(Auth)
		(*in).DeepCopyInto(*out)
	}
	if in.CreatePipelinePersistentStorage != nil {
		in, out := &in.CreatePipelinePersistentStorage, &out.CreatePipelinePersistentStorage
		*out = new(bool)
		**out = **in
	}
	if in.EnableWorkloadIdentity != nil {
		in, out := &in.EnableWorkloadIdentity, &out.EnableWorkloadIdentity
		*out = new(bool)
		**out = **in
	}
	if in.DeploymentManagerConfig != nil {
		in, out := &in.DeploymentManagerConfig, &out.DeploymentManagerConfig
		*out = new(DeploymentManagerConfig)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GcpPluginSpec.
func (in *GcpPluginSpec) DeepCopy() *GcpPluginSpec {
	if in == nil {
		return nil
	}
	out := new(GcpPluginSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *IAP) DeepCopyInto(out *IAP) {
	*out = *in
	if in.OAuthClientSecret != nil {
		in, out := &in.OAuthClientSecret, &out.OAuthClientSecret
		*out = new(v1beta1.SecretRef)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new IAP.
func (in *IAP) DeepCopy() *IAP {
	if in == nil {
		return nil
	}
	out := new(IAP)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *KfGcpPlugin) DeepCopyInto(out *KfGcpPlugin) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new KfGcpPlugin.
func (in *KfGcpPlugin) DeepCopy() *KfGcpPlugin {
	if in == nil {
		return nil
	}
	out := new(KfGcpPlugin)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *KfGcpPlugin) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
