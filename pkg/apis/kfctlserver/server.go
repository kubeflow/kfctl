package main

import (
	"context"
	"fmt"
	"net"

	"github.com/kubeflow/kfctl/v3/cmd/kfctl/cmd"
	api "github.com/kubeflow/kfctl/v3/pkg/apis/protos/kfctl"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// KfctlServer is used to register a server for the backend
type KfctlServer struct {
	api.UnimplementedKfctlServer
}

// Deploy takes in a KFDef and attempts to deploy Kubeflow to a Kubernetes cluster
func (k *KfctlServer) Deploy(ctx context.Context, deployer *api.KFDef) (*api.Status, error) {
	configFile := deployer.GetConfigFile()
	log.Infof("KFDef config to be deployed: %v", configFile)
	err := cmd.ApplyKFDef(configFile)
	status := &api.Status{}
	if err != nil {
		log.Errorf("error deploying Kubeflow: %v", err)
		status = &api.Status{Failure: true}
		return status, err
	}
	status = &api.Status{Success: true}
	return status, nil
}

func main() {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", 9090))
	if err != nil {
		log.Errorf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	api.RegisterKfctlServer(grpcServer, &KfctlServer{})

	if err = grpcServer.Serve(listener); err != nil {
		log.Errorf("Failed to serve: %v", err)
	}
}
