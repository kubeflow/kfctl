#**********************************************************************
# Builder
# 
# Create a go runtime suitable for building and testing kfctl
ARG GOLANG_VERSION=1.12.7
FROM golang:$GOLANG_VERSION as builder

RUN apt-get update
RUN apt-get install -y git unzip

# junit report is used to conver go test output to junit for reporting
RUN go get -u github.com/jstemmer/go-junit-report

# We need gcloud to get gke credentials.
RUN \
    cd /tmp && \
    wget -nv https://dl.google.com/dl/cloudsdk/release/install_google_cloud_sdk.bash && \
    chmod +x install_google_cloud_sdk.bash && \
    ./install_google_cloud_sdk.bash --disable-prompts --install-dir=/opt/

ENV PATH /go/bin:/usr/local/go/bin:/opt/google-cloud-sdk/bin:${PATH}

# Install kubectl
# We don't install via gcloud because we want the latest stable which is newer than what's in gcloud.
RUN  curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl && \
    mv kubectl /usr/local/bin && \
    chmod a+x /usr/local/bin/kubectl

# use go modules
ENV GO111MODULE=on
ENV GOPATH=/go

# Create kfctl folder
RUN mkdir -p ${GOPATH}/src/github.com/kubeflow/kfctl
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

RUN make install

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

CMD ["/bin/bash", "-c", "trap : TERM INT; sleep infinity & wait"]
