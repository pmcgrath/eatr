package main

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/pkg/api/v1" // corev1 "k8s.io/api/core/v1"     Waiting on the deps to settle, so reverted to what would work for clinet-go v4.0.0-beta.0
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	allNamespacesKey   = "**all-ns**" // Is not a valid namespace name so cannot clash with an existing namespace
	awsECRDNSPattern   = `^(?P<AccountId>\d{12})\.dkr\.ecr\.(?P<Region>.+)\.amazonaws\.com$`
	detailiedGLogLevel = 6
	secretDataTemplate = `{ "auths": { "%s": { "auth": "%s" } } }` // Docker config json file format, see ~/.docker/config.json
	queueName          = "eatr"
)

var (
	labelAndSecretNameRegEx = regexp.MustCompile(awsECRDNSPattern) // Will use this for the host namespace AWS credential secrets and to label the target namespaces to create ECR login tokens
)

type ecrInterface interface {
	GetAuthToken(ctx context.Context, region, id, secret string) (*ecr.AuthorizationData, error)
}

type k8sInterface interface {
	CreateSecret(string, *corev1.Secret) (*corev1.Secret, error)
	GetNamespace(string) (*corev1.Namespace, error)
	GetNamespaces() (*corev1.NamespaceList, error)
	GetSecret(string, string) (*corev1.Secret, error)
	GetSecrets(string) (*corev1.SecretList, error)
	UpdateSecret(string, *corev1.Secret) (*corev1.Secret, error)
}

type awsCredentials struct {
	Region string
	Id     string
	Secret string
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
		Help: "Number of secrets that have been created\\updated.",
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
			UpdateFunc: func(oldObj, newObj interface{}) {
				oldNS := oldObj.(*corev1.Namespace)
				newNS := newObj.(*corev1.Namespace)
				if oldNS.ResourceVersion != newNS.ResourceVersion {
					nsName := newNS.Name
					glog.V(detailiedGLogLevel).Infof("Updated ns [%s]\n", nsName)
					ctrl.Queue.Add(nsName)
				}
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
	nss, err := c.getNamespacesToProcess(key)
	if err != nil {
		return errors.Wrap(err, "get namespaces to process failed")
	}
	if len(nss) == 0 {
		glog.V(detailiedGLogLevel).Infoln("No namespaces to process")
		return nil
	}

	secretNames := c.getDistinctSecretNames(nss)
	authTokenDataMap, err := c.createECRAuthTokenData(secretNames)
	if err != nil {
		return errors.Wrap(err, "create ECR authorization token data failed")
	}
	if len(authTokenDataMap) == 0 {
		glog.V(detailiedGLogLevel).Infoln("No ECR authorization tokens created")
		return nil
	}

	for _, ns := range nss {
		for k, v := range ns.Labels {
			if labelAndSecretNameRegEx.MatchString(k) && v == "true" {
				if authTokenData, ok := authTokenDataMap[k]; ok {
					err = c.createNamespaceSecret(ns.Name, k, authTokenData)
					if err != nil {
						return errors.Wrapf(err, "create namespace [%s] secret [%s] failed", ns.Name, k)
					}
					c.SecretsCounter.Inc()
				} else {
					glog.V(detailiedGLogLevel).Infof("Skipping for namespace [%s] secret [%s], no ECR authorization token found\n", ns.Name, k)
				}
			}
		}
	}

	if key == allNamespacesKey {
		c.SecretRenewalsCounter.Inc()
	}

	glog.V(detailiedGLogLevel).Infoln("Completed renewing secrets")

	return nil
}

func (c *controller) getNamespacesToProcess(key string) ([]corev1.Namespace, error) {
	list := &corev1.NamespaceList{}
	if key == allNamespacesKey {
		glog.V(detailiedGLogLevel).Infoln("Getting namespaces")
		nsList, err := c.K8S.GetNamespaces()
		if err != nil {
			return nil, errors.Wrap(err, "get namespaces failed")
		}
		list = nsList
	} else {
		glog.V(detailiedGLogLevel).Infof("Getting namespace [%s]\n", key)
		ns, err := c.K8S.GetNamespace(key)
		if err != nil {
			return nil, errors.Wrapf(err, "get namespace [%s] failed", key)
		}

		list.Items = append(list.Items, *ns)
	}

	nss := []corev1.Namespace{}
	for _, ns := range list.Items {
		if ns.Status.Phase != corev1.NamespaceActive {
			continue
		}
		for k, v := range ns.Labels {
			if labelAndSecretNameRegEx.MatchString(k) && v == "true" {
				nss = append(nss, ns)
				break
			}
		}
	}

	return nss, nil
}

func (c *controller) getDistinctSecretNames(nss []corev1.Namespace) []string {
	set := stringset{}
	for _, ns := range nss {
		for k, v := range ns.Labels {
			if labelAndSecretNameRegEx.MatchString(k) && v == "true" {
				set[k] = empty{}
			}
		}
	}

	return set.Keys()
}

func (c *controller) createECRAuthTokenData(secretNames []string) (map[string]*ecr.AuthorizationData, error) {
	res := map[string]*ecr.AuthorizationData{}

	for _, secretName := range secretNames {
		glog.V(detailiedGLogLevel).Infof("Getting namespace [%s] secret [%s]\n", c.Config.HostNamespace, secretName)
		sec, err := c.K8S.GetSecret(c.Config.HostNamespace, secretName)
		if err != nil {
			if k8serr.IsNotFound(err) {
				glog.Infof("Namespace [%s] secret [%s] was not found, will skip\n", c.Config.HostNamespace, secretName)
				continue
			}
			return nil, errors.Wrapf(err, "get namespace [%s] secret [%s] failed", c.Config.HostNamespace, secretName)
		}

		region := string(sec.Data["aws_region"])
		id := string(sec.Data["aws_access_key_id"])
		secret := string(sec.Data["aws_secret_access_key"])
		maskedId := id

		glog.V(detailiedGLogLevel).Infof("Getting AWS ECR authorization token for region [%s] and access key id [%s]\n", region, maskedId)
		authTokenData, err := c.ECR.GetAuthToken(context.Background(), region, id, secret)
		if err != nil {
			return nil, errors.Wrapf(err, "get ECR authorization token failed for region [%s] and access key id [%s]", region, maskedId)
		}

		res[secretName] = authTokenData
	}

	return res, nil
}

func (c *controller) createNamespaceSecret(nsName, secretName string, authTokenData *ecr.AuthorizationData) error {
	endpoint := *(*authTokenData).ProxyEndpoint
	password := *(*authTokenData).AuthorizationToken
	secretData := []byte(fmt.Sprintf(secretDataTemplate, endpoint, password))

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: secretName,
		},
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: secretData,
		},
		Type: corev1.SecretTypeDockerConfigJson,
	}

	_, err := c.K8S.GetSecret(nsName, secretName)
	if err != nil {
		glog.V(detailiedGLogLevel).Infof("Creating namespace [%s] secret [%s]\n", nsName, secretName)
		_, err = c.K8S.CreateSecret(nsName, secret)
	} else {
		glog.V(detailiedGLogLevel).Infof("Updating namespace [%s] secret [%s]\n", nsName, secretName)
		_, err = c.K8S.UpdateSecret(nsName, secret)
	}
	if err != nil {
		return errors.Wrapf(err, "create or update of namespace [%s] secret [%s] failed", nsName, secretName)
	}

	return nil
}
