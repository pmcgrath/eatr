package main

import (
	"github.com/pkg/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/pkg/api/v1" // corev1 "k8s.io/api/core/v1"     Waiting on the deps to settle, so reverted to what would work for clinet-go v4.0.0-beta.0
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

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

func (k *k8sClient) GetActiveNamespaceNames() ([]string, error) {
	list, err := k.ClientSet.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "list k8s namespaces failed")
	}

	var nss []string
	for _, ns := range list.Items {
		if ns.Status.Phase != corev1.NamespaceActive {
			continue
		}

		nss = append(nss, ns.Name)
	}

	return nss, nil
}

func (k *k8sClient) GetSecret(ns, name string) (*corev1.Secret, error) {
	return k.ClientSet.CoreV1().Secrets(ns).Get(name, metav1.GetOptions{})
}

func (k *k8sClient) CreateSecret(ns string, s *corev1.Secret) (*corev1.Secret, error) {
	return k.ClientSet.CoreV1().Secrets(ns).Create(s)
}

func (k *k8sClient) UpdateSecret(ns string, s *corev1.Secret) (*corev1.Secret, error) {
	return k.ClientSet.CoreV1().Secrets(ns).Update(s)
}
