package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"k8s.io/client-go/informers"
)

// See	https://blog.heptio.com/straighten-out-your-kubernetes-client-go-dependencies-heptioprotip-8baeed46fe7d
//	https://github.com/coreos/prometheus-operator/blob/master/pkg/prometheus/operator.go
//	https://github.com/upmc-enterprises/registry-creds
//	https://github.com/jbeda/tgik-controller
//	https://engineering.bitnami.com/articles/a-deep-dive-into-kubernetes-controllers.html
//	https://github.com/skippbox/kubewatch/
//	https://github.com/aaronlevy/kube-controller-demo
//	https://github.com/heptio/ark
var (
	version     = "NotSet"
	repoBranch  = "NotSet"
	repoVersion = "NotSet"
)

func main() {
	if err := runMain(); err != nil {
		glog.Error(err.Error())
		os.Exit(2)
	}
}

func runMain() error {
	defer glog.Flush()

	ctx, cancel := context.WithCancel(context.Background())

	config, err := getConfig(os.Args)
	if err != nil {
		return errors.Wrap(err, "getConfig failed")
	}

	glog.Infof("Starting Version=%s Branch=%s RepoVersion=%s\n", version, repoBranch, repoVersion)
	glog.Infof("Starting listener on port %d\n", config.Port)
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", config.Port))
	if err != nil {
		return errors.Wrap(err, "listener failed")
	}

	glog.Infoln("Newing up k8s client")
	k8sClient, err := newK8sClient(config.KubeConfigFilePath)
	if err != nil {
		return errors.Wrap(err, "newK8sClient failed")
	}

	glog.Infoln("Newing up ECR")
	ecr := newECRClient()

	glog.Infoln("Newing up shared informer factory and namesapce informer")
	informersFactory := informers.NewSharedInformerFactory(k8sClient.ClientSet, config.InformersResyncInterval)
	nsInformer := informersFactory.Core().V1().Namespaces()

	glog.Infoln("Getting prometheus registry and gatherer - defaults")
	promRegistry := prometheus.DefaultRegisterer.(*prometheus.Registry)
	promGatherer := prometheus.DefaultGatherer

	glog.Infoln("Newing up controller")
	controller, err := newController(config, k8sClient, nsInformer.Informer(), promRegistry, ecr)
	if err != nil {
		return errors.Wrap(err, "newController failure")
	}

	glog.Infoln("Newing up diagnostic HTTP server")
	srv := newDiagnosticHTTPServer(promGatherer)

	glog.Infoln("Starting informers factory")
	informersFactory.Start(ctx.Done())

	glog.Infoln("Starting controller go routine")
	go func() {
		controller.Run(ctx.Done())
		glog.Infoln("Controller run completed")
	}()

	glog.Infoln("Starting diagnostic HTTP server go routine")
	// PENDING:
	go func() error {
		err := srv.Serve(listener)
		if err != http.ErrServerClosed {
			return errors.Wrap(err, "HTTP serve failed")
		}
		glog.Infoln("HTTP serve completed")
		return nil
	}()

	glog.Infoln("Starting diagnostic HTTP server gracefull shutdown go routine")
	// PENDING:
	go func() error {
		<-ctx.Done()
		glog.Infoln("Shutting down HTTP server")
		if err := srv.Shutdown(context.Background()); err != nil {
			return errors.Wrap(err, "HTTP server shutdown failed")
		}
		glog.Infoln("HTTP server shutdown completed")
		return nil
	}()

	glog.Infoln("Waiting...")
	term := make(chan os.Signal)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)
	<-term
	cancel()

	glog.Infof("Allowing %s to shutdown\n", config.ShutdownGracePeriod)
	time.Sleep(config.ShutdownGracePeriod)
	glog.Infoln("Done")

	return nil
}

func newDiagnosticHTTPServer(promGatherer prometheus.Gatherer) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(promGatherer, promhttp.HandlerOpts{}))
	mux.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
	mux.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	mux.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	mux.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	mux.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))

	return &http.Server{Handler: mux}
}
