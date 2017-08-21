package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

// See	https://blog.heptio.com/straighten-out-your-kubernetes-client-go-dependencies-heptioprotip-8baeed46fe7d
//	https://github.com/coreos/prometheus-operator/blob/master/pkg/prometheus/operator.go
//	https://github.com/upmc-enterprises/registry-creds
//	https://github.com/jbeda/tgik-controller
const (
	DefaultAuthenticationTokenRenewalInterval = 6 * time.Hour
	DefaultInformersResyncInterval            = 5 * time.Minute
	DefaultNamespaceBlacklist                 = "ci-cd, default, kube-public, kube-system, monitoring"
	DefaultPort                               = 5000
	DefaultShutdownGracePeriod                = 3 * time.Second
)

var (
	version     = "1.0"
	repoBranch  = "NotSet"
	repoVersion = "NotSet"
)

type config struct {
	AuthenticationTokenRenewalInterval time.Duration
	InformersResyncInterval            time.Duration
	KubeConfigFilePath                 string
	NamespaceBlacklist                 string
	NamespaceBlacklistSet              map[string]struct{}
	Port                               int
	SecretName                         string
	ShutdownGracePeriod                time.Duration
}

type controller struct {
	Config                config
	Client                *kubernetes.Clientset
	NamespaceListerSynced cache.InformerSynced
	GetECRAuthToken       func(context.Context) (*ecr.AuthorizationData, error)
}

func main() {
	log.Println("Getting config")
	config, err := getConfig()
	dieIfErr(err)

	log.Printf("Starting listener on port %d\n", config.Port)
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", config.Port))
	dieIfErr(err)

	log.Println("Newing up k8s client")
	client, err := newK8sClient(config.KubeConfigFilePath)
	dieIfErr(err)

	log.Println("Newing up controller")
	informersFactory := informers.NewSharedInformerFactory(client, config.InformersResyncInterval)
	controller, err := newController(client, config, informersFactory, getECRAuthToken)
	dieIfErr(err)

	log.Println("Newing up diagnostic HTTP server")
	srv := newDiagnosticHTTPServer()

	ctx, cancel := context.WithCancel(context.Background())
	term := make(chan os.Signal)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)

	log.Println("Starting informers factory")
	informersFactory.Start(ctx.Done())

	log.Println("Starting controller go routine")
	go func() {
		controller.Run(ctx.Done())
		log.Println("Controller run completed")
	}()

	log.Println("Starting diagnostic HTTP server go routine")
	go func() {
		err := srv.Serve(listener)
		if err != http.ErrServerClosed {
			dieIfErr(errors.Wrap(err, "HTTP serve failed"))
		}
		log.Println("HTTP serve completed")
	}()

	log.Println("Starting diagnostic HTTP server gracefull shutdown go routine")
	go func() {
		<-ctx.Done()
		log.Println("Shutting down HTTP server")
		if err := srv.Shutdown(context.Background()); err != nil {
			dieIfErr(errors.Wrap(err, "HTTP server shutdown failed"))
		}
		log.Println("HTTP server shutdown completed")
	}()

	log.Println("Waiting...")
	<-term
	cancel()

	log.Printf("Allowing %s to shutdown\n", config.ShutdownGracePeriod)
	time.Sleep(config.ShutdownGracePeriod)
	log.Println("Done")
}

func getConfig() (config, error) {
	config := config{
		AuthenticationTokenRenewalInterval: DefaultAuthenticationTokenRenewalInterval,
		InformersResyncInterval:            DefaultInformersResyncInterval,
		KubeConfigFilePath:                 os.Getenv("KUBECONFIG"),
		NamespaceBlacklist:                 DefaultNamespaceBlacklist,
		Port:                               DefaultPort,
		ShutdownGracePeriod:                DefaultShutdownGracePeriod,
	}

	flag.DurationVar(&config.AuthenticationTokenRenewalInterval, "auth-token-renewal-interval", config.AuthenticationTokenRenewalInterval, "Authentication token renewal interval - ECR tokens expire after 12 hours so should be less")
	flag.DurationVar(&config.InformersResyncInterval, "informers-resync-interval", config.InformersResyncInterval, "Shared informers resync interval")
	flag.StringVar(&config.KubeConfigFilePath, "config-file-path", config.KubeConfigFilePath, "Kube config file pathi, optional, only used for testing outside the cluster, can also set the KUBECONFIG env var")
	flag.StringVar(&config.NamespaceBlacklist, "namespace-blacklist", config.NamespaceBlacklist, "Namespace blacklist (comma seperated list)")
	flag.IntVar(&config.Port, "port", config.Port, "Port to surface diagnostics on")
	flag.StringVar(&config.SecretName, "secret-name", config.SecretName, "Secret name (Optional - If left empty will use the registry domain name)")
	flag.DurationVar(&config.ShutdownGracePeriod, "shutdown-grace-period", config.ShutdownGracePeriod, "Shutdown grace period")
	flag.Parse()

	config.NamespaceBlacklistSet = make(map[string]struct{})
	for _, nsName := range strings.Split(config.NamespaceBlacklist, ",") {
		config.NamespaceBlacklistSet[strings.TrimSpace(nsName)] = struct{}{}
	}

	return config, nil
}

func dieIfErr(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		os.Exit(2)
	}
}

func newK8sClient(configFilePath string) (*kubernetes.Clientset, error) {
	var config *rest.Config
	var err error
	if configFilePath != "" {
		config, err = clientcmd.BuildConfigFromFlags("", configFilePath)
	} else {
	}
	if err != nil {
		return nil, errors.Wrap(err, "create k8s client failed")
	}
	client := kubernetes.NewForConfigOrDie(config)

	return client, nil
}

func newController(client *kubernetes.Clientset, config config, informersFactory informers.SharedInformerFactory, getECRAuthData func(context.Context) (*ecr.AuthorizationData, error)) (*controller, error) {
	nsInformer := informersFactory.Core().V1().Namespaces()

	ctrl := &controller{
		Config:                config,
		Client:                client,
		NamespaceListerSynced: nsInformer.Informer().HasSynced,
		GetECRAuthToken:       getECRAuthToken,
	}

	nsInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				ctrl.onNSAdd(obj.(*corev1.Namespace))
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				ctrl.onNSUpdate(oldObj.(*corev1.Namespace), newObj.(*corev1.Namespace))
			},
		},
	)

	return ctrl, nil
}

func (c *controller) Run(stop <-chan struct{}) {
	log.Println("Controller.Run: Waiting for cache sync")
	if !cache.WaitForCacheSync(stop, c.NamespaceListerSynced) {
		log.Print("Controller.Run: Timed out waiting for cache sync")
		return
	}
	log.Println("Controller.Run: Caches are synced")

	tick := time.Tick(c.Config.AuthenticationTokenRenewalInterval)
	for {
		log.Println("Controller.Run: Renewing ECR image pull secrets")
		if err := c.renewECRImagePullSecrets(); err != nil {
			log.Printf("Controller.Run: Renew ECR image pull secrets error: %s\n", err)
			runtime.HandleError(err)
		}

		select {
		case <-tick:
		case <-stop:
			log.Println("Controller.Run: Received stop signal, exiting loop")
			return
		}
	}
}

func (c *controller) onNSAdd(ns *corev1.Namespace) {
	log.Printf("Controller.onNSAdd: %s\n", ns.GetName())
}

func (c *controller) onNSUpdate(oldNs, newNs *corev1.Namespace) {
	log.Printf("Controller.onNSUpdate: %s   %s\n", oldNs.GetName(), newNs.GetName())
}

func (c *controller) renewECRImagePullSecrets() error {
	const secretDataTemplate = `{"%s": { "username":"oauth2accesstoken", "password":"%s", "email":"none" } }`

	log.Println("Controller.renewECRImagePullSecrets: Getting AWS ECR authorization token")
	authTokenData, err := c.GetECRAuthToken(context.Background())
	if err != nil {
		return errors.Wrap(err, "get ECR authorization token failed")
	}

	log.Println("Controller.renewECRImagePullSecrets: Getting active namespace names")
	nsNames, err := c.getActiveNamespaceNames()
	if err != nil {
		return errors.Wrap(err, "get active namespace names failed")
	}

	endpoint := *(*authTokenData).ProxyEndpoint
	password := *(*authTokenData).AuthorizationToken

	secretName := c.Config.SecretName
	if secretName == "" {
		secretName = strings.TrimPrefix(endpoint, "https://")
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: secretName,
		},
		Data: map[string][]byte{
			corev1.DockerConfigKey: []byte(fmt.Sprintf(secretDataTemplate, endpoint, password)),
		},
		Type: corev1.SecretTypeDockercfg,
	}

	for _, nsName := range nsNames {
		if _, ok := c.Config.NamespaceBlacklistSet[nsName]; ok {
			log.Printf("Controller.renewECRImagePullSecrets: Skipping [%s] namespace\n", nsName)
			continue
		}

		if _, err = c.Client.CoreV1().Secrets(nsName).Get(secret.Name, metav1.GetOptions{}); err != nil {
			log.Printf("Controller.renewECRImagePullSecrets: Creating secret [%s] in [%s] namespace\n", secret.Name, nsName)
			_, err = c.Client.CoreV1().Secrets(nsName).Create(secret)
		} else {
			log.Printf("Controller.renewECRImagePullSecrets: Updating secret [%s] in [%s] namespace\n", secret.Name, nsName)
			_, err = c.Client.CoreV1().Secrets(nsName).Update(secret)
		}
		if err != nil {
			return errors.Wrapf(err, "create or update of secret [%s] in [%s] namespace failed", secret.Name, nsName)
		}
	}
	log.Println("Controller.renewECRImagePullSecrets: Completed")

	return nil
}

func (c *controller) getActiveNamespaceNames() ([]string, error) {
	list, err := c.Client.CoreV1().Namespaces().List(metav1.ListOptions{})
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

func getECRAuthToken(ctx context.Context) (*ecr.AuthorizationData, error) {
	svc := ecr.New(session.New())

	inp := &ecr.GetAuthorizationTokenInput{}
	out, err := svc.GetAuthorizationTokenWithContext(ctx, inp)
	if err != nil {
		return nil, errors.Wrap(err, "get authorization token failed")
	}

	return out.AuthorizationData[0], nil
}

func getActiveK8sNamespaces(ctx context.Context, configFilePath string) ([]string, error) {
	client, err := newK8sClient(configFilePath)
	if err != nil {
		return nil, errors.Wrap(err, "create k8s client failed")
	}

	list, err := client.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "list k8s namespaces failed")
	}

	var nss []string
	for _, ns := range list.Items {
		if ns.Status.Phase == corev1.NamespaceActive {
			nss = append(nss, ns.Name)
		}
	}

	return nss, nil
}

func newDiagnosticHTTPServer() *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
	mux.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	mux.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	mux.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	mux.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))

	return &http.Server{Handler: mux}
}
