module github.com/kubeflow/kfctl/v3

require (
	cloud.google.com/go v0.38.0
	github.com/aws/aws-sdk-go v1.15.78
	github.com/cenkalti/backoff v2.1.1+incompatible
	github.com/deckarep/golang-set v1.7.1
	github.com/ghodss/yaml v1.0.0
	github.com/go-kit/kit v0.8.0
	github.com/gogo/protobuf v1.2.1
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.3.1
	github.com/hashicorp/go-getter v1.0.2
	github.com/imdario/mergo v0.3.7
	github.com/kubeflow/kubeflow/bootstrap/v3 v3.0.0-20190823213626-a301e6be1341
	github.com/kubeflow/kubeflow/components/profile-controller v0.0.0-20190614045418-7ca3cfb39368
	github.com/mitchellh/go-homedir v1.1.0
	github.com/onrik/logrus v0.2.1
	github.com/otiai10/copy v1.0.1
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v0.9.2
	github.com/prometheus/common v0.2.0
	github.com/sirupsen/logrus v1.3.0
	github.com/spf13/afero v1.2.2
	github.com/spf13/cobra v0.0.3
	github.com/spf13/viper v1.3.1
	golang.org/x/crypto v0.0.0
	golang.org/x/net v0.0.0-20190503192946-f4e77d36d62c
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	golang.org/x/time v0.0.0-20181108054448-85acf8d2951c
	google.golang.org/api v0.6.0
	google.golang.org/genproto v0.0.0-20190502173448-54afdca5d873
	gopkg.in/src-d/go-git.v4 v4.12.0
	k8s.io/api v0.0.0-20190222213804-5cb15d344471
	k8s.io/apiextensions-apiserver v0.0.0-20190228180357-d002e88f6236
	k8s.io/apimachinery v0.0.0-20190221213512-86fb29eff628
	k8s.io/client-go v0.0.0-20190228174230-b40b2a5939e4
	sigs.k8s.io/controller-runtime v0.1.12
	sigs.k8s.io/kustomize v2.0.3+incompatible
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v9.1.0+incompatible
	github.com/Sirupsen/logrus => github.com/sirupsen/logrus v1.0.5
	github.com/go-openapi/jsonpointer => github.com/go-openapi/jsonpointer v0.17.0
	github.com/go-openapi/jsonreference => github.com/go-openapi/jsonreference v0.17.0
	github.com/go-openapi/spec => github.com/go-openapi/spec v0.18.0
	github.com/go-openapi/swag => github.com/go-openapi/swag v0.17.0
	github.com/kubeflow/kubeflow/components/profile-controller => github.com/kubeflow/kubeflow/components/profile-controller v0.0.0-20190614045418-7ca3cfb39368
	github.com/mitchellh/go-homedir => github.com/mitchellh/go-homedir v1.0.0
	github.com/russross/blackfriday => github.com/russross/blackfriday v1.5.2-0.20180428102519-11635eb403ff // indirect
	golang.org/x/crypto => golang.org/x/crypto v0.0.0-20181203042331-505ab145d0a9
	k8s.io/api => k8s.io/api v0.0.0-20190222213804-5cb15d344471
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20190228180357-d002e88f6236
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190221213512-86fb29eff628
	k8s.io/apiserver => k8s.io/apiserver v0.0.0-20190228174905-79427f02047f
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.0.0-20190228180923-a9e421a79326
	k8s.io/client-go => k8s.io/client-go v0.0.0-20190228174230-b40b2a5939e4
	k8s.io/kubernetes => k8s.io/kubernetes v1.13.4
	sigs.k8s.io/application => sigs.k8s.io/application v0.0.0-20190404151855-67ae7f915d4e
	sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.1.12
	sigs.k8s.io/kustomize => sigs.k8s.io/kustomize v2.0.3+incompatible
)
