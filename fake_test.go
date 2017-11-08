package main

import (
	"context"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecr"

	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

// ECR client fake
type FakeECRClient struct {
	DomainName     string
	GetAuthTokenFn func(context.Context, string, string, string) (*ecr.AuthorizationData, error)
}

func NewFakeECRClient() *FakeECRClient {
	f := &FakeECRClient{DomainName: "account.ecr.aws.com"}

	f.GetAuthTokenFn = func(ctx context.Context, region, id, secret string) (*ecr.AuthorizationData, error) {
		return &ecr.AuthorizationData{
			AuthorizationToken: aws.String("SomeAuthTokenJibberish"),
			ExpiresAt:          aws.Time(time.Now().Add(12 * time.Hour)),
			ProxyEndpoint:      aws.String("https://" + f.DomainName),
		}, nil
	}

	return f
}

func (f *FakeECRClient) GetAuthToken(ctx context.Context, region, id, secret string) (*ecr.AuthorizationData, error) {
	return f.GetAuthTokenFn(ctx, region, id, secret)
}

// K8S client fake
type FakeK8SClient struct {
	mutex              sync.RWMutex
	namespaces         *corev1.NamespaceList
	secrets            *corev1.SecretList
	createdSecretCount int
	updatedSecretCount int

	CreateSecretFn  func(string, *corev1.Secret) (*corev1.Secret, error)
	GetNamespaceFn  func(string) (*corev1.Namespace, error)
	GetNamespacesFn func() (*corev1.NamespaceList, error)
	GetSecretFn     func(string, string) (*corev1.Secret, error)
	GetSecretsFn    func(string) (*corev1.SecretList, error)
	UpdateSecretFn  func(string, *corev1.Secret) (*corev1.Secret, error)
}

func NewFakeK8SClient(nsNames, inactiveNsNames []string) *FakeK8SClient {
	const indexNotFound = -1
	k8sNotFoundErr := &k8serr.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonNotFound}}

	f := &FakeK8SClient{
		namespaces: &corev1.NamespaceList{Items: make([]corev1.Namespace, len(nsNames))},
		secrets:    &corev1.SecretList{},
	}

	getSecretIndexFn := func(ns, name string) int {
		for i, c := range f.secrets.Items {
			if c.Namespace == ns && c.Name == name {
				return i
			}
		}
		return indexNotFound
	}

	getNamespaceIndexFn := func(name string) int {
		for i, c := range f.namespaces.Items {
			if c.Name == name {
				return i
			}
		}
		return indexNotFound
	}

	for i, nsName := range nsNames {
		f.namespaces.Items[i] = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: nsName, Namespace: nsName},
			Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
		}
	}
	for _, inactiveNsName := range inactiveNsNames {
		if idx := getNamespaceIndexFn(inactiveNsName); idx != indexNotFound {
			f.namespaces.Items[idx].Status = corev1.NamespaceStatus{Phase: corev1.NamespaceActive}
		}
	}

	f.CreateSecretFn = func(ns string, s *corev1.Secret) (*corev1.Secret, error) {
		f.mutex.Lock()
		defer f.mutex.Unlock()

		s.ObjectMeta.Namespace = ns
		f.secrets.Items = append(f.secrets.Items, *s)
		f.createdSecretCount++

		return s, nil
	}

	f.GetNamespaceFn = func(ns string) (*corev1.Namespace, error) {
		f.mutex.RLock()
		defer f.mutex.RUnlock()

		idx := getNamespaceIndexFn(ns)
		if idx == indexNotFound {
			return nil, k8sNotFoundErr
		}

		return &f.namespaces.Items[idx], nil // PENDING: Mutation - can I use DeepClone
	}

	f.GetNamespacesFn = func() (*corev1.NamespaceList, error) {
		f.mutex.RLock()
		defer f.mutex.RUnlock()

		return f.namespaces, nil // PENDING: Mutation - can I use DeepClone
	}

	f.GetSecretFn = func(ns, name string) (*corev1.Secret, error) {
		f.mutex.RLock()
		defer f.mutex.RUnlock()

		idx := getSecretIndexFn(ns, name)
		if idx == indexNotFound {
			return nil, k8sNotFoundErr
		}

		return &f.secrets.Items[idx], nil // PENDING: Mutation - can I use DeepClone
	}

	f.GetSecretsFn = func(ns string) (*corev1.SecretList, error) {
		f.mutex.RLock()
		defer f.mutex.RUnlock()

		return f.secrets, nil
	}

	f.UpdateSecretFn = func(ns string, s *corev1.Secret) (*corev1.Secret, error) {
		f.mutex.Lock()
		defer f.mutex.Unlock()

		idx := getSecretIndexFn(ns, s.Name)
		if idx == indexNotFound {
			return nil, k8sNotFoundErr
		}

		f.secrets.Items[idx] = s
		f.updatedSecretCount++

		return s, nil
	}

	return f
}

func (f *FakeK8SClient) CreateSecret(ns string, s *corev1.Secret) (*corev1.Secret, error) {
	return f.CreateSecretFn(ns, s)
}

func (f *FakeK8SClient) GetNamespace(ns string) (*corev1.Namespace, error) {
	return f.GetNamespaceFn(ns)
}

func (f *FakeK8SClient) GetNamespaces() (*corev1.NamespaceList, error) {
	return f.GetNamespacesFn()
}

func (f *FakeK8SClient) GetSecret(ns, name string) (*corev1.Secret, error) {
	return f.GetSecretFn(ns, name)
}

func (f *FakeK8SClient) GetSecrets(ns string) (*corev1.SecretList, error) {
	return f.GetSecretsFn(ns)
}

func (f *FakeK8SClient) UpdateSecret(ns string, s *corev1.Secret) (*corev1.Secret, error) {
	return f.UpdateSecretFn(ns, s)
}

func (f *FakeK8SClient) getSecretKey(ns, name string) string {
	return ns + "***" + name
}

/*
func (f *FakeK8SClient) Secrets() []*corev1.Secret {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	ss := make([]*corev1.Secret, len(f.secrets), len(f.secrets))
	i := 0
	for _, v := range f.secrets {
		ss[i] = v
		i++
	}

	return ss
}
*/

func (f *FakeK8SClient) CreatedSecretCount() int {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	return f.createdSecretCount
}

func (f *FakeK8SClient) UpdatedSecretCount() int {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	return f.updatedSecretCount
}

// Shared informer fake - Must satisfy the client-go/tools/cache/SharedInformer interface
type FakeSharedInformer struct {
	mutex   sync.RWMutex
	handler cache.ResourceEventHandler
}

func NewFakeSharedInformer() *FakeSharedInformer {
	return &FakeSharedInformer{}
}

func (f *FakeSharedInformer) AddEventHandler(handler cache.ResourceEventHandler) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.handler = handler
}

func (f *FakeSharedInformer) AddEventHandlerWithResyncPeriod(handler cache.ResourceEventHandler, resyncPeriod time.Duration) {
}

func (f *FakeSharedInformer) GetStore() cache.Store {
	// Satisfy interface
	return nil
}

func (f *FakeSharedInformer) GetController() cache.Controller {
	// Satisfy interface
	return nil
}

func (f *FakeSharedInformer) Run(stopCh <-chan struct{}) {
	// Satisfy interface
}

func (f *FakeSharedInformer) HasSynced() bool {
	// Satisfy interface
	return true
}

func (f *FakeSharedInformer) LastSyncResourceVersion() string {
	// Satisfy interface
	return ""
}

func (f *FakeSharedInformer) SimulateAddNamespace(name string) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	f.handler.OnAdd(ns)
}
