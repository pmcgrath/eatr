package main

import (
	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Subset so we can test, we can fake the subset of ClientSet that the controller needs
type k8sClient struct {
	ClientSet *kubernetes.Clientset
}

func newK8sClient(configFilePath string) (*k8sClient, error) {
	var config *rest.Config
	var err error
	if configFilePath != "" {
		config, err = clientcmd.BuildConfigFromFlags("", configFilePath)
	} else {
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, errors.Wrap(err, "create k8s client failed")
	}

	clientSet := kubernetes.NewForConfigOrDie(config)

	return &k8sClient{ClientSet: clientSet}, nil
}

func (k *k8sClient) CreateSecret(ns string, s *corev1.Secret) (*corev1.Secret, error) {
	return k.ClientSet.CoreV1().Secrets(ns).Create(s)
}

func (k *k8sClient) GetNamespace(name string) (*corev1.Namespace, error) {
	return k.ClientSet.CoreV1().Namespaces().Get(name, metav1.GetOptions{})
}

func (k *k8sClient) GetNamespaces() (*corev1.NamespaceList, error) {
	return k.ClientSet.CoreV1().Namespaces().List(metav1.ListOptions{})
}

func (k *k8sClient) GetSecret(ns, name string) (*corev1.Secret, error) {
	return k.ClientSet.CoreV1().Secrets(ns).Get(name, metav1.GetOptions{})
}

func (k *k8sClient) GetSecrets(ns string) (*corev1.SecretList, error) {
	return k.ClientSet.CoreV1().Secrets(ns).List(metav1.ListOptions{})
}

func (k *k8sClient) UpdateSecret(ns string, s *corev1.Secret) (*corev1.Secret, error) {
	return k.ClientSet.CoreV1().Secrets(ns).Update(s)
}
