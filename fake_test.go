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
	"k8s.io/apimachinery/pkg/util/sets"
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

// Seed data to initialise a FakeK8sClient
type FakeK8SClientSeedNamespace struct {
	Name     string
	IsActive bool
	Labels   map[string]string
	Secrets  []string
}

// K8S client fake, also has some extra helpers and state tracking for tests
type FakeK8SClient struct {
	mutex                      sync.RWMutex
	namespaces                 *corev1.NamespaceList
	secrets                    *corev1.SecretList
	createdNamespaceSecretKeys sets.String
	newlyCreatedSecretCount    int
	updatedSecretCount         int

	CreateSecretFn  func(string, *corev1.Secret) (*corev1.Secret, error)
	GetNamespaceFn  func(string) (*corev1.Namespace, error)
	GetNamespacesFn func() (*corev1.NamespaceList, error)
	GetSecretFn     func(string, string) (*corev1.Secret, error)
	GetSecretsFn    func(string) (*corev1.SecretList, error)
	UpdateSecretFn  func(string, *corev1.Secret) (*corev1.Secret, error)
}

func NewFakeK8SClient(seed []FakeK8SClientSeedNamespace) *FakeK8SClient {
	const indexNotFound = -1
	k8sNotFoundErr := &k8serr.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonNotFound}}

	f := &FakeK8SClient{
		namespaces:                 &corev1.NamespaceList{},
		secrets:                    &corev1.SecretList{},
		createdNamespaceSecretKeys: sets.NewString(),
	}

	for _, seedNS := range seed {
		phase := corev1.NamespaceActive
		if !seedNS.IsActive {
			phase = corev1.NamespaceTerminating
		}

		f.namespaces.Items = append(f.namespaces.Items,
			corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: seedNS.Name, Namespace: seedNS.Name, Labels: seedNS.Labels}, Status: corev1.NamespaceStatus{Phase: phase}})

		for _, secretName := range seedNS.Secrets {
			// We don't need a type or data for our tests
			f.secrets.Items = append(f.secrets.Items,
				corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: seedNS.Name}})
		}
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

	f.CreateSecretFn = func(ns string, s *corev1.Secret) (*corev1.Secret, error) {
		f.mutex.Lock()
		defer f.mutex.Unlock()
		s.ObjectMeta.Namespace = ns
		f.secrets.Items = append(f.secrets.Items, *s)
		f.createdNamespaceSecretKeys[ns+":"+(*s).Name] = sets.Empty{}
		f.newlyCreatedSecretCount++

		return s, nil
	}

	f.GetNamespaceFn = func(ns string) (*corev1.Namespace, error) {
		f.mutex.RLock()
		defer f.mutex.RUnlock()

		idx := getNamespaceIndexFn(ns)
		if idx == indexNotFound {
			return nil, k8sNotFoundErr
		}

		return f.namespaces.Items[idx].DeepCopy(), nil
	}

	f.GetNamespacesFn = func() (*corev1.NamespaceList, error) {
		f.mutex.RLock()
		defer f.mutex.RUnlock()

		return f.namespaces.DeepCopy(), nil
	}

	f.GetSecretFn = func(ns, name string) (*corev1.Secret, error) {
		f.mutex.RLock()
		defer f.mutex.RUnlock()

		idx := getSecretIndexFn(ns, name)
		if idx == indexNotFound {
			return nil, k8sNotFoundErr
		}

		return f.secrets.Items[idx].DeepCopy(), nil
	}

	f.GetSecretsFn = func(ns string) (*corev1.SecretList, error) {
		f.mutex.RLock()
		defer f.mutex.RUnlock()

		ss := &corev1.SecretList{}
		for _, s := range f.secrets.Items {
			if s.Namespace == ns {
				ss.Items = append(ss.Items, s)
			}
		}
		return ss, nil
	}

	f.UpdateSecretFn = func(ns string, s *corev1.Secret) (*corev1.Secret, error) {
		f.mutex.Lock()
		defer f.mutex.Unlock()

		idx := getSecretIndexFn(ns, s.Name)
		if idx == indexNotFound {
			return nil, k8sNotFoundErr
		}

		f.secrets.Items[idx] = *s.DeepCopy()
		f.createdNamespaceSecretKeys[ns+":"+(*s).Name] = sets.Empty{}
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

// Insert new namespace record - used for populating the local cache with no counter increments - post initialization - needed to test post start new namesapce handling
func (f *FakeK8SClient) InsertNewNamespaceRecord(ns *corev1.Namespace) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	// Have assumed it is not a duplicate - lets be optimistic for tests
	f.namespaces.Items = append(f.namespaces.Items, *ns.DeepCopy())
}

// Update namespace record - used for altering the local cache with no counter increments - post initialization - needed to test post start updated namesapce handling
func (f *FakeK8SClient) UpdateNamespaceRecord(ns *corev1.Namespace) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	// Have not handled no match - lets be optimistic for tests
	for i, c := range f.namespaces.Items {
		if c.Name == (*ns).Name {
			f.namespaces.Items[i] = *ns
			return
		}
	}
}

func (f *FakeK8SClient) NewlyCreatedSecretCount() int {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	return f.newlyCreatedSecretCount
}

func (f *FakeK8SClient) UpdatedSecretCount() int {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	return f.updatedSecretCount
}

// Total secrets created - newly created + existing secrets that were updated
func (f *FakeK8SClient) TotalSecretsCreated() int {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	return f.newlyCreatedSecretCount + f.updatedSecretCount
}

func (f *FakeK8SClient) DistinctNamespacedSecretKeysCreated() string {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	r := ""
	for i, k := range f.createdNamespaceSecretKeys.List() {
		if i == 0 {
			r = k
			continue
		}
		r += "," + k
	}

	return r
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

func (f *FakeSharedInformer) SimulateAddNamespace(ns *corev1.Namespace) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.handler.OnAdd(ns.DeepCopy())
}

func (f *FakeSharedInformer) SimulateUpdateNamespace(oldNS, newNS *corev1.Namespace) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.handler.OnUpdate(oldNS.DeepCopy(), newNS.DeepCopy())
}
