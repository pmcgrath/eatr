package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
//	https://engineering.bitnami.com/articles/a-deep-dive-into-kubernetes-controllers.html
const (
	DefaultAuthenticationTokenRenewalInterval = 6 * time.Hour
	DefaultInformersResyncInterval            = 5 * time.Minute
	DefaultNamespaceBlacklist                 = "ci-cd, default, kube-public, kube-system, monitoring"
	DefaultLoggingVerbosityLevel              = 0
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
	LoggingVerbosityLevel              int
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
	config, err := getConfig(os.Args)
	dieIfErr(err)

	glog.Infof("Starting listener on port %d\n", config.Port)
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", config.Port))
	dieIfErr(err)

	glog.Infoln("Newing up k8s client")
	client, err := newK8sClient(config.KubeConfigFilePath)
	dieIfErr(err)

	glog.Infoln("Newing up controller")
	informersFactory := informers.NewSharedInformerFactory(client, config.InformersResyncInterval)
	controller, err := newController(client, config, informersFactory, getECRAuthToken)
	dieIfErr(err)

	glog.Infoln("Newing up diagnostic HTTP server")
	srv := newDiagnosticHTTPServer()

	ctx, cancel := context.WithCancel(context.Background())
	term := make(chan os.Signal)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)

	glog.Infoln("Starting informers factory")
	informersFactory.Start(ctx.Done())

	glog.Infoln("Starting controller go routine")
	go func() {
		controller.Run(ctx.Done())
		glog.Infoln("Controller run completed")
	}()

	glog.Infoln("Starting diagnostic HTTP server go routine")
	go func() {
		err := srv.Serve(listener)
		if err != http.ErrServerClosed {
			dieIfErr(errors.Wrap(err, "HTTP serve failed"))
		}
		glog.Infoln("HTTP serve completed")
	}()

	glog.Infoln("Starting diagnostic HTTP server gracefull shutdown go routine")
	go func() {
		<-ctx.Done()
		glog.Infoln("Shutting down HTTP server")
		if err := srv.Shutdown(context.Background()); err != nil {
			dieIfErr(errors.Wrap(err, "HTTP server shutdown failed"))
		}
		glog.Infoln("HTTP server shutdown completed")
	}()

	glog.Infoln("Waiting...")
	<-term
	cancel()

	glog.Infof("Allowing %s to shutdown\n", config.ShutdownGracePeriod)
	time.Sleep(config.ShutdownGracePeriod)
	glog.Infoln("Done")
}

func getConfig(args []string) (config, error) {
	config := config{
		AuthenticationTokenRenewalInterval: DefaultAuthenticationTokenRenewalInterval,
		InformersResyncInterval:            DefaultInformersResyncInterval,
		KubeConfigFilePath:                 os.Getenv("KUBECONFIG"),
		LoggingVerbosityLevel:              DefaultLoggingVerbosityLevel,
		NamespaceBlacklist:                 DefaultNamespaceBlacklist,
		Port:                               DefaultPort,
		ShutdownGracePeriod:                DefaultShutdownGracePeriod,
	}

	// Using an explicit flagset so we do not mix the glog flags via the client-go package
	fs := flag.NewFlagSet(args[0], flag.ExitOnError)
	fs.DurationVar(&config.AuthenticationTokenRenewalInterval, "auth-token-renewal-interval", config.AuthenticationTokenRenewalInterval, "Authentication token renewal interval - ECR tokens expire after 12 hours so should be less")
	fs.DurationVar(&config.InformersResyncInterval, "informers-resync-interval", config.InformersResyncInterval, "Shared informers resync interval")
	fs.StringVar(&config.KubeConfigFilePath, "config-file-path", config.KubeConfigFilePath, "Kube config file pathi, optional, only used for testing outside the cluster, can also set the KUBECONFIG env var")
	fs.IntVar(&config.LoggingVerbosityLevel, "logging-verbosity-level", config.LoggingVerbosityLevel, "Logging verbosity level, can set to 6 or higher to get debug level logs, will also see client-go logs")
	fs.StringVar(&config.NamespaceBlacklist, "namespace-blacklist", config.NamespaceBlacklist, "Namespace blacklist (comma seperated list)")
	fs.IntVar(&config.Port, "port", config.Port, "Port to surface diagnostics on")
	fs.StringVar(&config.SecretName, "secret-name", config.SecretName, "Secret name (Optional - If left empty will use the registry domain name)")
	fs.DurationVar(&config.ShutdownGracePeriod, "shutdown-grace-period", config.ShutdownGracePeriod, "Shutdown grace period")
	fs.Parse(args[1:])

	// Limited glog config
	// See https://stackoverflow.com/questions/28207226/how-do-i-set-the-log-directory-of-glog-from-cod://stackoverflow.com/questions/28207226/how-do-i-set-the-log-directory-of-glog-from-code
	// Simulate global flags so we can configure some of the glog flags
	// Need to add global flags as the defaul is to exit on error - i.e. Unknown flags which is how our flags above will be seen
	fs.VisitAll(func(f *flag.Flag) { _ = flag.String((*f).Name, "", "") })
	flag.Lookup("logtostderr").Value.Set("true")
	flag.Lookup("v").Value.Set(strconv.Itoa(config.LoggingVerbosityLevel))
	flag.Parse()

	config.NamespaceBlacklistSet = make(map[string]struct{})
	for _, nsName := range strings.Split(config.NamespaceBlacklist, ",") {
		config.NamespaceBlacklistSet[strings.TrimSpace(nsName)] = struct{}{}
	}

	return config, nil
}

func dieIfErr(err error) {
	if err != nil {
		glog.Error(err.Error())
		os.Exit(2)
	}
}

func newK8sClient(configFilePath string) (*kubernetes.Clientset, error) {
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
				// Added to cache for first time
				ctrl.onNSAdd(obj.(*corev1.Namespace))
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				// Updated existing cache item - updates and informer resyncs
				ctrl.onNSUpdate(oldObj.(*corev1.Namespace), newObj.(*corev1.Namespace))
			},
		},
	)

	return ctrl, nil
}

func (c *controller) Run(stop <-chan struct{}) {
	// PENDING: Should I we fail to connect to the cluster ? So subject this to a timeout
	glog.Infoln("Controller.Run: Waiting for cache sync")
	if !cache.WaitForCacheSync(stop, c.NamespaceListerSynced) {
		glog.Infoln("Controller.Run: Timed out waiting for cache sync")
		return
	}
	glog.Infoln("Controller.Run: Caches are synced")

	tick := time.Tick(c.Config.AuthenticationTokenRenewalInterval)
	for {
		glog.V(6).Infoln("Controller.Run: Renewing ECR image pull secrets")
		if err := c.renewECRImagePullSecrets(); err != nil {
			glog.Errorf("Controller.Run: Renew ECR image pull secrets error: %s\n", err)
		}

		select {
		case <-tick:
		case <-stop:
			glog.Infoln("Controller.Run: Received stop signal, exiting loop")
			return
		}
	}
}

func (c *controller) onNSAdd(ns *corev1.Namespace) {
	glog.V(6).Infof("Controller.onNSAdd: %s\n", ns.GetName())
}

func (c *controller) onNSUpdate(oldNs, newNs *corev1.Namespace) {
	glog.V(6).Infof("Controller.onNSUpdate: %s   %s\n", oldNs.GetName(), newNs.GetName())
}

func (c *controller) renewECRImagePullSecrets() error {
	const secretDataTemplate = `{ "auths": { "%s": { "auth": "%s" } } }` // Docker config json file format, see ~/.docker/config.json

	glog.V(6).Infoln("Controller.renewECRImagePullSecrets: Getting AWS ECR authorization token")
	authTokenData, err := c.GetECRAuthToken(context.Background())
	if err != nil {
		return errors.Wrap(err, "get ECR authorization token failed")
	}

	glog.V(6).Infoln("Controller.renewECRImagePullSecrets: Getting active namespace names")
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
			corev1.DockerConfigJsonKey: []byte(fmt.Sprintf(secretDataTemplate, endpoint, password)),
		},
		Type: corev1.SecretTypeDockerConfigJson,
	}

	for _, nsName := range nsNames {
		if _, ok := c.Config.NamespaceBlacklistSet[nsName]; ok {
			glog.V(6).Infof("Controller.renewECRImagePullSecrets: Skipping [%s] namespace\n", nsName)
			continue
		}

		if _, err = c.Client.CoreV1().Secrets(nsName).Get(secret.Name, metav1.GetOptions{}); err != nil {
			glog.V(6).Infof("Controller.renewECRImagePullSecrets: Creating secret [%s] in [%s] namespace\n", secret.Name, nsName)
			_, err = c.Client.CoreV1().Secrets(nsName).Create(secret)
		} else {
			glog.V(6).Infof("Controller.renewECRImagePullSecrets: Updating secret [%s] in [%s] namespace\n", secret.Name, nsName)
			_, err = c.Client.CoreV1().Secrets(nsName).Update(secret)
		}
		if err != nil {
			return errors.Wrapf(err, "create or update of secret [%s] in [%s] namespace failed", secret.Name, nsName)
		}
	}
	glog.V(6).Infoln("Controller.renewECRImagePullSecrets: Completed")

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
		return nil, errors.Wrap(err, "get ECR authorization token failed")
	}

	return out.AuthorizationData[0], nil
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
