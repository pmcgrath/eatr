package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	allNamespacesKey   = "**all-ns**" // Is not a valid namespace name so cannot clash with an existing namespace
	detailiedGLogLevel = 6
	secretDataTemplate = `{ "auths": { "%s": { "auth": "%s" } } }` // Docker config json file format, see ~/.docker/config.json
	queueName          = "eatr"
)

type ecrInterface interface {
	GetAuthToken(ctx context.Context) (*ecr.AuthorizationData, error)
}

type k8sInterface interface {
	CreateSecret(string, *corev1.Secret) (*corev1.Secret, error)
	GetActiveNamespaceNames() ([]string, error)
	GetSecret(string, string) (*corev1.Secret, error)
	UpdateSecret(string, *corev1.Secret) (*corev1.Secret, error)
}

type controller struct {
	Config                config
	K8S                   k8sInterface
	NamespaceListerSynced cache.InformerSynced
	Queue                 workqueue.RateLimitingInterface
	ECR                   ecrInterface
	SecretsCounter        prometheus.Counter
	SecretRenewalsCounter prometheus.Counter
}

func newController(config config, k8sClient k8sInterface, informer cache.SharedInformer, prometheusRegistry *prometheus.Registry, ecrClient ecrInterface) (*controller, error) {
	secretsCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "secrets_created_total",
		Help: "Number of secrets that have been created.",
	})
	secretRenewalsCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "secret_renewals_total",
		Help: "Number of secret renewals made.",
	})
	prometheusRegistry.MustRegister(secretsCounter)
	prometheusRegistry.MustRegister(secretRenewalsCounter)

	ctrl := &controller{
		Config: config,
		K8S:    k8sClient,
		NamespaceListerSynced: informer.HasSynced,
		Queue:                 workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), queueName),
		ECR:                   ecrClient,
		SecretsCounter:        secretsCounter,
		SecretRenewalsCounter: secretRenewalsCounter,
	}

	informer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				nsName := (obj.(*corev1.Namespace)).Name
				glog.V(detailiedGLogLevel).Infof("Added ns [%s]\n", nsName)
				ctrl.Queue.Add(nsName)
			},
		},
	)

	return ctrl, nil
}

func (c *controller) Run(stop <-chan struct{}) {
	defer c.Queue.ShutDown()

	// PENDING: Should we fail if we can't connect to the cluster ? So subject this to a timeout
	glog.Infoln("Waiting for cache sync")
	if !cache.WaitForCacheSync(stop, c.NamespaceListerSynced) {
		glog.Infoln("Timed out waiting for cache sync")
		return
	}
	glog.Infoln("Caches are synced")

	glog.Infoln("Starting queue consumer loop")
	go c.runQueueConsumerLoop()

	tick := time.Tick(c.Config.AuthenticationTokenRenewalInterval)
	for {
		// First population will be via the Informers AddFunc
		select {
		case <-tick:
			glog.Infoln("Adding queue key to renew for all namespaces")
			c.Queue.Add(allNamespacesKey)
		case <-stop:
			glog.Infoln("Received stop signal, exiting loop")
			return
		}
	}
}

func (c *controller) runQueueConsumerLoop() {
	for {
		key, quit := c.Queue.Get()
		if quit {
			glog.Infoln("Run queue consumer loop is done")
			return
		}

		skey := key.(string)
		glog.V(detailiedGLogLevel).Infof("Processing queue item [%s]\n", skey)
		if err := c.renewECRImagePullSecrets(skey); err != nil {
			// Not going to bother with retrying, could do with c.Queue.AddRateLimited(key)
			glog.Errorf("Renew ECR image pull secrets error: %s\n", err)
		}

		c.Queue.Forget(key)
		c.Queue.Done(key)
	}
}

func (c *controller) renewECRImagePullSecrets(key string) error {
	glog.V(detailiedGLogLevel).Infoln("Getting AWS ECR authorization token")
	authTokenData, err := c.ECR.GetAuthToken(context.Background())
	if err != nil {
		return errors.Wrap(err, "get ECR authorization token failed")
	}

	nsNames := []string{key}
	if key == allNamespacesKey {
		glog.V(detailiedGLogLevel).Infoln("Getting active namespace names")
		nsNames, err = c.K8S.GetActiveNamespaceNames()
		if err != nil {
			return errors.Wrap(err, "get active namespace names failed")
		}
	}

	endpoint := *(*authTokenData).ProxyEndpoint
	password := *(*authTokenData).AuthorizationToken

	secretName := c.Config.SecretName
	if secretName == "" {
		secretName = strings.TrimPrefix(endpoint, "https://")
	}

	secretData := []byte(fmt.Sprintf(secretDataTemplate, endpoint, password))

	for _, nsName := range nsNames {
		if _, ok := c.Config.NamespaceBlacklistSet[nsName]; ok {
			glog.V(detailiedGLogLevel).Infof("Skipping [%s] namespace\n", nsName)
			continue
		}

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: secretName,
			},
			Data: map[string][]byte{
				corev1.DockerConfigJsonKey: secretData,
			},
			Type: corev1.SecretTypeDockerConfigJson,
		}

		if _, err = c.K8S.GetSecret(nsName, secret.Name); err != nil {
			glog.V(detailiedGLogLevel).Infof("Creating secret [%s] in [%s] namespace\n", secret.Name, nsName)
			_, err = c.K8S.CreateSecret(nsName, secret)
		} else {
			glog.V(detailiedGLogLevel).Infof("Updating secret [%s] in [%s] namespace\n", secret.Name, nsName)
			_, err = c.K8S.UpdateSecret(nsName, secret)
		}
		if err != nil {
			return errors.Wrapf(err, "create or update of secret [%s] in [%s] namespace failed", secret.Name, nsName)
		}

		c.SecretsCounter.Inc()
	}

	if key == allNamespacesKey {
		c.SecretRenewalsCounter.Inc()
	}

	glog.V(detailiedGLogLevel).Infoln("Completed renewing secrets")

	return nil
}
