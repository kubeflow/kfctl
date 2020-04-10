package aws

import (
	kfapis "github.com/kubeflow/kfctl/v3/pkg/apis"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path/filepath"
)

// getK8sclient creates a Kubernetes client set
func getK8sclient() (*clientset.Clientset, error) {
	kubeconfig := os.Getenv("KUBECONFIG")

	if kubeconfig == "" {
		if home := homeDir(); home != "" {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
	}

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, errors.Errorf("Failed to create config file from %s", kubeconfig)
	}

	clientset, err := clientset.NewForConfig(config)
	if err != nil {
		return nil, errors.Errorf("Failed to create kubernetes clientset")
	}

	return clientset, nil
}

// homeDir returns home folder and it's used to detect kubeconfig file
func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func createNamespace(client *clientset.Clientset, namespace string) error {
	_, err := client.CoreV1().Namespaces().Get(namespace, metav1.GetOptions{})
	if err == nil {
		log.Infof("Namespace %v already exists...", namespace)
		return nil
	}
	log.Infof("Creating namespace: %v", namespace)
	_, err = client.CoreV1().Namespaces().Create(
		&v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		},
	)

	return err
}

func deleteNamespace(client *clientset.Clientset, namespace string) error {
	_, err := client.CoreV1().Namespaces().Get(namespace, metav1.GetOptions{})
	if err != nil {
		log.Infof("Namespace %v does not exist, skip deleting", namespace)
		return nil
	}
	log.Infof("Deleting namespace: %v", namespace)
	background := metav1.DeletePropagationBackground
	err = client.CoreV1().Namespaces().Delete(
		namespace, &metav1.DeleteOptions{
			PropagationPolicy: &background,
		},
	)

	return err
}

func createSecret(client *clientset.Clientset, secretName string, namespace string, data map[string][]byte) error {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: data,
	}
	log.Infof("Creating secret: %v/%v", namespace, secretName)
	_, err := client.CoreV1().Secrets(namespace).Create(secret)
	if err == nil {
		return nil
	} else {
		return &kfapis.KfError{
			Code:    int(kfapis.INTERNAL_ERROR),
			Message: err.Error(),
		}
	}
}
