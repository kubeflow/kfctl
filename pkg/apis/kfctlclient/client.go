// Sample kfctl client with Go
package main

import (
	"context"

	api "github.com/kubeflow/kfctl/v3/pkg/apis/protos/kfctl"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

func main() {
	grpcConnection, err := grpc.Dial(":9090", grpc.WithInsecure())
	if err != nil {
		log.Printf("Could not connect to gRPC server at port 9090: %v", err)
		return
	}
	defer grpcConnection.Close()

	client := api.NewKfctlClient(grpcConnection)

	response, err := client.Deploy(context.Background(), &api.KFDef{ConfigFile: "kfctl_k8s_istio.yaml"})
	if err != nil {
		log.Printf("Error deploying Kubeflow for config: %v", err)
		return
	}
	log.Printf("response from kfctl serverr: %v", response.GetSuccess())
}
