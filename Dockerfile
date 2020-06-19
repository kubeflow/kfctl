#**********************************************************************
# Builder
# Create a go runtime suitable for building and testing kfctl
ARG GOLANG_VERSION=1.13.7
FROM golang:$GOLANG_VERSION as builder

ARG BRANCH=master
ARG REPO=https://github.com/kubeflow/kubeflow

RUN apt-get update
RUN apt-get install -y git unzip jq vim

# junit report is used to conver go test output to junit for reporting
RUN go get -u github.com/jstemmer/go-junit-report

# We need gcloud to get gke credentials.
RUN if [ "$(uname -m)" = "x86_64" ]; then \
        cd /tmp && \
        wget -nv https://dl.google.com/dl/cloudsdk/release/install_google_cloud_sdk.bash && \
        chmod +x install_google_cloud_sdk.bash && \
        ./install_google_cloud_sdk.bash --disable-prompts --install-dir=/opt/; \
    fi

ENV PATH /go/bin:/usr/local/go/bin:/opt/google-cloud-sdk/bin:${PATH}

# use go modules
ENV GO111MODULE=on
ENV GOPATH=/go
# Workaround for https://github.com/kubernetes/gengo/issues/146
ENV GOROOT=/usr/local/go

# Create kfctl folder
RUN mkdir -p ${GOPATH}/src/github.com/kubeflow/kfctl
WORKDIR ${GOPATH}/src/github.com/kubeflow
RUN mkdir kubeflow
RUN echo REPO=${REPO} branch=${BRANCH}
RUN git clone ${REPO} --depth=1 --branch ${BRANCH} --single-branch kubeflow
WORKDIR ${GOPATH}/src/github.com/kubeflow/kfctl

# Download dependencies first to optimize Docker caching.
COPY go.mod .
COPY go.sum .
RUN go mod download
# Copy in the source
COPY . .

#**********************************************************************
#
# kfctl_base
#
FROM builder as kfctl_base

RUN make build-kfctl && \
    if [ "$(uname -m)" = "aarch64" ]; then \
        cp bin/arm64/kfctl bin/kfctl; \
    fi

#**********************************************************************
#
# Final image base
#

FROM alpine:3.10.1 as barebones_base
RUN mkdir -p /opt/kubeflow
WORKDIR /opt/kubeflow

#**********************************************************************
#
# kfctl
#
FROM barebones_base as kfctl

COPY --from=kfctl_base /go/src/github.com/kubeflow/kfctl/bin/kfctl /usr/local/bin
COPY --from=kfctl_base /go/src/github.com/kubeflow/kfctl/third_party /third_party
COPY --from=kfctl_base /go/pkg/mod /third_party/vendor


CMD ["/bin/bash", "-c", "trap : TERM INT; sleep infinity & wait"]
