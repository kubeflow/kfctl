# Copyright 2017 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
GCLOUD_PROJECT ?= kubeflow-images-public
GOLANG_VERSION ?= 1.12.7
GOPATH ?= $(HOME)/go
# To build without the cache set the environment variable
# export DOCKER_BUILD_OPTS=--no-cache
KFCTL_IMG ?= gcr.io/$(GCLOUD_PROJECT)/kfctl
TAG ?= $(eval TAG := $(shell git describe --tags --long --always))$(TAG)
REPO ?= $(shell echo $$(cd ../kubeflow && git config --get remote.origin.url) | sed 's/git@\(.*\):\(.*\).git$$/https:\/\/\1\/\2/')
BRANCH ?= $(shell cd ../kubeflow && git branch | grep '^*' | awk '{print $$2}')
KFCTL_TARGET ?= kfctl
MOUNT_KUBE ?=  -v $(HOME)/.kube:/root/.kube
MOUNT_GCP ?=  -v $(HOME)/.config:/root/.config
# set to -V
VERBOSE ?=
PLUGINS_ENVIRONMENT ?= $(GOPATH)/src/github.com/kubeflow/kfctl/bin
export GO111MODULE = on
export GO = go
ARCH ?= $(shell ${GO} env|grep GOOS|cut -d'=' -f2|tr -d '"')

# Location of junit file
JUNIT_FILE ?= /tmp/report.xml

%.so:
	cd cmd/plugins/$* && \
	${GO} build -i -gcflags '-N -l' -o ../../../bin/$*.so -buildmode=plugin $*.go

%.init:
	@echo kfctl init test/$* $(VERBOSE) --platform $* --project $(GCLOUD_PROJECT) --version master && \
	PLUGINS_ENVIRONMENT=$(PLUGINS_ENVIRONMENT) kfctl init $(PWD)/test/$* $(VERBOSE) --platform $* --project $(GCLOUD_PROJECT) --version master

%.init-no-platform:
	@echo kfctl init test/$* $(VERBOSE) --version master && \
	kfctl init $(PWD)/test/$* $(VERBOSE) --version master

%.generate:
	@echo kfctl generate all $(VERBOSE) '(--platform '$*')' && \
	cd test/$* && \
	PLUGINS_ENVIRONMENT=$(PLUGINS_ENVIRONMENT) kfctl generate all $(VERBOSE) --mount-local --email gcp-deploy@$(GCLOUD_PROJECT).iam.gserviceaccount.com

%.md:

all: build

auth:
	gcloud auth configure-docker

# Run go fmt against code
fmt:
	@${GO} fmt ./config ./cmd/... ./pkg/...

# Run go vet against code
vet:
	@${GO} vet ./config ./cmd/... ./pkg/...

generate:
	@${GO} generate ./config ./pkg/apis/apps/kfdef/... ./pkg/utils/... ./pkg/kfapp/minikube ./pkg/kfapp/gcp/... ./cmd/kfctl/...

${GOPATH}/bin/deepcopy-gen:
	GO111MODULE=off ${GO} get k8s.io/code-generator/cmd/deepcopy-gen

config/zz_generated.deepcopy.go: config/types.go
	${GOPATH}/bin/deepcopy-gen -h hack/boilerplate.go.txt -i github.com/kubeflow/kfctl/v3/config -O zz_generated.deepcopy \
	-p config

pkg/apis/apps/kfdef/v1alpha1/zz_generated.deepcopy.go: pkg/apis/apps/kfdef/v1alpha1/application_types.go
	${GOPATH}/bin/deepcopy-gen -h hack/boilerplate.go.txt -i github.com/kubeflow/kfctl/v3/pkg/apis/apps/kfdef/... -O zz_generated.deepcopy \
		-p pkg/apis/apps/kfdef/v1alpha1/

pkg/apis/apps/kfdef/v1beta1/zz_generated.deepcopy.go: pkg/apis/apps/kfdef/v1beta1/application_types.go
	${GOPATH}/bin/deepcopy-gen -h hack/boilerplate.go.txt -i github.com/kubeflow/kfctl/v3/pkg/apis/apps/kfdef/... -O zz_generated.deepcopy \
		-p pkg/apis/apps/kfdef/v1beta1/

pkg/apis/apps/kfdef/v1/zz_generated.deepcopy.go: pkg/apis/apps/kfdef/v1/application_types.go
	${GOPATH}/bin/deepcopy-gen -h hack/boilerplate.go.txt -i github.com/kubeflow/kfctl/v3/pkg/apis/apps/kfdef/... -O zz_generated.deepcopy \
		-p pkg/apis/apps/kfdef/v1/

pkg/apis/apps/plugins/gcp/v1alpha1/zz_generated.deepcopy.go: pkg/apis/apps/plugins/gcp/v1alpha1/types.go
	${GOPATH}/bin/deepcopy-gen -h hack/boilerplate.go.txt -i github.com/kubeflow/kfctl/v3/pkg/apis/apps/plugins/gcp/... -O zz_generated.deepcopy \
		-p pkg/apis/apps/plugins/gcp/v1alpha1/

pkg/apis/apps/plugins/aws/v1alpha1/zz_generated.deepcopy.go: pkg/apis/apps/plugins/aws/v1alpha1/types.go
	${GOPATH}/bin/deepcopy-gen -h hack/boilerplate.go.txt -i github.com/kubeflow/kfctl/v3/pkg/apis/apps/plugins/aws/... -O zz_generated.deepcopy \
		-p pkg/apis/apps/plugins/aws/v1alpha1/

pkg/kfconfig/zz_generated.deepcopy.go: pkg/kfconfig/types.go
	${GOPATH}/bin/deepcopy-gen -h hack/boilerplate.go.txt -i github.com/kubeflow/kfctl/v3/pkg/kfconfig/... -O zz_generated.deepcopy \
		-p pkg/kfconfig/

pkg/kfconfig/awsplugin/zz_generated.deepcopy.go: pkg/kfconfig/awsplugin/types.go
	${GOPATH}/bin/deepcopy-gen -h hack/boilerplate.go.txt -i github.com/kubeflow/kfctl/v3/pkg/kfconfig/awsplugin/... -O zz_generated.deepcopy \
		-p pkg/kfconfig/awsplugin/

pkg/kfconfig/gcpplugin/zz_generated.deepcopy.go: pkg/kfconfig/gcpplugin/types.go
	${GOPATH}/bin/deepcopy-gen -h hack/boilerplate.go.txt -i github.com/kubeflow/kfctl/v3/pkg/kfconfig/gcpplugin/... -O zz_generated.deepcopy\
		-p pkg/kfconfig/gcpplugin/

deepcopy: ${GOPATH}/bin/deepcopy-gen config/zz_generated.deepcopy.go \
	pkg/apis/apps/kfdef/v1alpha1/zz_generated.deepcopy.go \
	pkg/apis/apps/kfdef/v1beta1/zz_generated.deepcopy.go \
	pkg/apis/apps/kfdef/v1/zz_generated.deepcopy.go \
	pkg/apis/apps/plugins/gcp/v1alpha1/zz_generated.deepcopy.go \
	pkg/apis/apps/plugins/aws/v1alpha1/zz_generated.deepcopy.go \
	pkg/kfconfig/zz_generated.deepcopy.go \
	pkg/kfconfig/awsplugin/zz_generated.deepcopy.go \
	pkg/kfconfig/gcpplugin/zz_generated.deepcopy.go

build: build-kfctl

build-kfctl: deepcopy generate fmt vet
	# TODO(swiftdiaries): figure out import conflict errors for windows
	#GOOS=windows GOARCH=amd64 ${GO} build -gcflags '-N -l' -ldflags "-X main.VERSION=$(TAG)" -o bin/windows/kfctl.exe cmd/kfctl/main.go
	GOOS=darwin GOARCH=amd64 ${GO} build -gcflags '-N -l' -ldflags "-X main.VERSION=${TAG}" -o bin/darwin/kfctl cmd/kfctl/main.go
	GOOS=linux GOARCH=amd64 ${GO} build -gcflags '-N -l' -ldflags "-X main.VERSION=$(TAG)" -o bin/linux/kfctl cmd/kfctl/main.go
	cp bin/$(ARCH)/kfctl bin/kfctl

# Release tarballs suitable for upload to GitHub release pages
build-kfctl-tgz: build-kfctl
	chmod a+rx ./bin/kfctl
	rm -f bin/*.tgz
	cd bin/linux && tar -cvzf kfctl_$(TAG)_linux.tar.gz ./kfctl
	cd bin/darwin && tar -cvzf kfctl_${TAG}_darwin.tar.gz ./kfctl

# push the releases to a GitHub page
push-to-github-release: build-kfctl-tgz
	github-release upload \
	    --user kubeflow \
	    --repo kubeflow \
	    --tag $(TAG) \
	    --name "kfctl_$(TAG)_linux.tar.gz" \
	    --file bin/kfctl_$(TAG)_linux.tar.gz
	github-release upload \
	    --user kubeflow \
	    --repo kubeflow \
	    --tag $(TAG) \
	    --name "kfctl_$(TAG)_darwin.tar.gz" \
	    --file bin/kfctl_$(TAG)_darwin.tar.gz

build-kfctl-container:
	DOCKER_BUILDKIT=1 docker build \
                --build-arg REPO="$(REPO)" \
                --build-arg BRANCH=$(BRANCH) \
		--build-arg GOLANG_VERSION=$(GOLANG_VERSION) \
		--build-arg VERSION=$(TAG) \
		--target=$(KFCTL_TARGET) \
		--tag $(KFCTL_IMG)/builder:$(TAG) .
	@echo Built $(KFCTL_IMG)/builder:$(TAG)
	mkdir -p bin
	docker create \
		--name=temp_kfctl_container \
		$(KFCTL_IMG)/builder:$(TAG)
	docker cp temp_kfctl_container:/usr/local/bin/kfctl ./bin/kfctl
	docker rm temp_kfctl_container
	@echo Exported kfctl binary to bin/kfctl

# build containers using GCLOUD_PROJECT
build-gcb:
	gcloud --project=$(GCLOUD_PROJECT)\
		builds submit \
		--machine-type=n1-highcpu-32 \
		--substitutions=TAG_NAME=$(TAG)
		--config=cloudbuild.yaml .


# Build but don't attach the latest tag. This allows manual testing/inspection of the image
# first.
push: build
	docker push $(BOOTSTRAPPER_IMG):$(TAG)
	@echo Pushed $(BOOTSTRAPPER_IMG):$(TAG)

push-latest: push
	gcloud container images add-tag --quiet $(BOOTSTRAPPER_IMG):$(TAG) $(BOOTSTRAPPER_IMG):latest --verbosity=info
	echo created $(BOOTSTRAPPER_IMG):latest

push-kfctl-container: build-kfctl-container
	docker push $(KFCTL_IMG):$(TAG)
	@echo Pushed $(KFCTL_IMG):$(TAG)

push-kfctl-container-latest: push-kfctl-container
	gcloud container images add-tag --quiet $(KFCTL_IMG):$(TAG) $(KFCTL_IMG):latest --verbosity=info
	@echo created $(KFCTL_IMG):latest

install: build-kfctl dockerfordesktop.so
	@echo copying bin/kfctl to /usr/local/bin
	@cp bin/kfctl /usr/local/bin

run-kfctl-container: build-kfctl-container
	docker run $(MOUNT_KUBE) $(MOUNT_GCP) --entrypoint /bin/sh -it $(KFCTL_IMG):$(TAG)

#***************************************************************************************************
# Build a docker container that can be used to build kfctl
#
# The rules in this section are used to build the docker image that provides
# a suitable go build environment for kfctl

build-builder-container:
	docker build \
		--build-arg GOLANG_VERSION=$(GOLANG_VERSION) \
		--target=builder \
		--tag $(KFCTL_IMG):$(TAG) .
	@echo Built $(KFCTL_IMG):$(TAG)

# build containers using GCLOUD_PROJECT
build-builder-container-gcb:
	gcloud --project=$(GCLOUD_PROJECT) \
		builds submit \
		--machine-type=n1-highcpu-32 \
		--substitutions=TAG_NAME=$(TAG),_TARGET=builder \
		--config=cloudbuild.yaml .

#***************************************************************************************************

clean:
	rm -rf test && mkdir test

doc:
	doctoc ./cmd/kfctl/README.md README.md k8sSpec/README.md developer_guide.md


#**************************************************************************************************
# checks licenses
check-licenses:
	./third_party/check-license.sh
# rules to run unittests
#
test: build-kfctl check-licenses
	go test ./... -v


# Run the unittests and output a junit report for use with prow
test-junit: build-kfctl
	echo Running tests ... junit_file=$(JUNIT_FILE)
	go test ./... -v 2>&1 | go-junit-report > $(JUNIT_FILE) --set-exit-code

#***************************************************************************************************
test-init: clean install dockerfordesktop.init minikube.init gcp.init none.init-no-platform

test-generate: test-init dockerfordesktop.generate minikube.generate gcp.generate none.generate

test-apply: test-generate dockerfordesktop.apply minikube.apply gcp.apply none.apply

