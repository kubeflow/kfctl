# kfctl

kfctl is the control plane for  for deploying and managing Kubeflow delployments. The primary supported mode of deoployment is to use [kfctl as a CLI](https://github.com/kubeflow/kfctl/tree/master/cmd/kfctl) which can be used with KFDef configurations for different cloud configurations to deploy and manage Kubeflow. Please also look at the docs on [Kubeflow website](https://www.kubeflow.org/docs/started/getting-started/) for deployments options for different Cloud providers

Additionally, we have also introduced [Kubeflow Operator](./operator.md) in incubation mode, which apart from deploying Kubeflow, will perform additional fucntionalities around monitoring the deployment for consistency etc. 
