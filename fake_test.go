package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/pkg/api/v1" // corev1 "k8s.io/api/core/v1"     Waiting on the deps to settle, so reverted to what would work for clinet-go v4.0.0-beta.0
	"k8s.io/client-go/tools/cache"
)

// ECR client fake
type FakeECRClient struct {
	DomainName     string
	GetAuthTokenFn func(context.Context) (*ecr.AuthorizationData, error)
}

func NewFakeECRClient() *FakeECRClient {
	f := &FakeECRClient{DomainName: "account.ecr.aws.com"}

	f.GetAuthTokenFn = func(ctx context.Context) (*ecr.AuthorizationData, error) {
		return &ecr.AuthorizationData{
			AuthorizationToken: aws.String("SomeAuthTokenJibberish"),
			ExpiresAt:          aws.Time(time.Now().Add(12 * time.Hour)),
			ProxyEndpoint:      aws.String("https://" + f.DomainName),
		}, nil
	}

	return f
}

func (f *FakeECRClient) GetAuthToken(ctx context.Context) (*ecr.AuthorizationData, error) {
	return f.GetAuthTokenFn(ctx)
}

// K8S client fake
type FakeK8SClient struct {
	mutex              sync.RWMutex
	activeNamespaces   []string
	secrets            map[string]*corev1.Secret
	createdSecretCount int
	updatedSecretCount int

	GetActiveNamespaceNamesFn func() ([]string, error)
	GetSecretFn               func(string, string) (*corev1.Secret, error)
	CreateSecretFn            func(string, *corev1.Secret) (*corev1.Secret, error)
	UpdateSecretFn            func(string, *corev1.Secret) (*corev1.Secret, error)
}

func NewFakeK8SClient(initialActiveNamespaces []string) *FakeK8SClient {
	f := &FakeK8SClient{
		activeNamespaces: initialActiveNamespaces,
		secrets:          make(map[string]*corev1.Secret),
	}

	f.GetActiveNamespaceNamesFn = func() ([]string, error) {
		f.mutex.RLock()
		defer f.mutex.RUnlock()

		return f.activeNamespaces, nil
	}
	f.GetSecretFn = func(ns, name string) (*corev1.Secret, error) {
		f.mutex.RLock()
		defer f.mutex.RUnlock()

		if s, ok := f.secrets[f.getSecretKey(ns, name)]; ok {
			return s, nil
		} else {
			return nil, errors.New("Not found")
		}
	}
	f.CreateSecretFn = func(ns string, s *corev1.Secret) (*corev1.Secret, error) {
		f.mutex.Lock()
		defer f.mutex.Unlock()

		f.secrets[f.getSecretKey(ns, s.Name)] = s
		f.createdSecretCount++
		return s, nil
	}
	f.UpdateSecretFn = func(ns string, s *corev1.Secret) (*corev1.Secret, error) {
		f.mutex.Lock()
		defer f.mutex.Unlock()

		f.secrets[f.getSecretKey(ns, s.Name)] = s
		f.updatedSecretCount++
		return s, nil
	}

	return f
}

func (f *FakeK8SClient) getSecretKey(ns, name string) string {
	return ns + "***" + name
}

func (f *FakeK8SClient) GetActiveNamespaceNames() ([]string, error) {
	return f.GetActiveNamespaceNamesFn()
}

func (f *FakeK8SClient) GetSecret(ns, name string) (*corev1.Secret, error) {
	return f.GetSecretFn(ns, name)
}

func (f *FakeK8SClient) CreateSecret(ns string, s *corev1.Secret) (*corev1.Secret, error) {
	return f.CreateSecretFn(ns, s)
}

func (f *FakeK8SClient) UpdateSecret(ns string, s *corev1.Secret) (*corev1.Secret, error) {
	return f.UpdateSecretFn(ns, s)
}

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
