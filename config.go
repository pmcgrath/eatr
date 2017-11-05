package main

import (
	"flag"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAuthenticationTokenRenewalInterval = 6 * time.Hour
	defaultHostNamespace                      = "ci-cd"
	defaultInformersResyncInterval            = 5 * time.Minute
	defaultNamespaceBlacklist                 = defaultHostNamespace + ", default, kube-public, kube-system, monitoring"
	defaultLoggingVerbosityLevel              = 0
	defaultPort                               = 5000
	defaultShutdownGracePeriod                = 3 * time.Second
)

type empty struct{}

type stringset map[string]empty

func (set stringset) Keys() []string {
	ss := make([]string, len(set))
	i := 0
	for k, _ := range set {
		ss[i] = k
		i++
	}

	return ss
}

type config struct {
	AuthenticationTokenRenewalInterval time.Duration
	HostNamespace                      string
	InformersResyncInterval            time.Duration
	KubeConfigFilePath                 string
	LoggingVerbosityLevel              int
	NamespaceBlacklist                 string
	NamespaceBlacklistSet              stringset
	Port                               int
	ShutdownGracePeriod                time.Duration
}

func getConfig(args []string) (config, error) {
	config := getDefaultConfig()

	// Using an explicit flagset so we do not mix the glog flags via the client-go package
	fs := flag.NewFlagSet(args[0], flag.ExitOnError)
	fs.DurationVar(&config.AuthenticationTokenRenewalInterval, "auth-token-renewal-interval", config.AuthenticationTokenRenewalInterval, "Authentication token renewal interval - ECR tokens expire after 12 hours so should be less")
	fs.StringVar(&config.HostNamespace, "host-namespace", config.HostNamespace, "Host namespace")
	fs.DurationVar(&config.InformersResyncInterval, "informers-resync-interval", config.InformersResyncInterval, "Shared informers resync interval")
	fs.StringVar(&config.KubeConfigFilePath, "config-file-path", config.KubeConfigFilePath, "Kube config file path, optional, only used for testing outside the cluster, can also set the KUBECONFIG env var")
	fs.IntVar(&config.LoggingVerbosityLevel, "logging-verbosity-level", config.LoggingVerbosityLevel, "Logging verbosity level, can set to 6 or higher to get debug level logs, will also see client-go logs")
	fs.StringVar(&config.NamespaceBlacklist, "namespace-blacklist", config.NamespaceBlacklist, "Namespace blacklist (comma seperated list)")
	fs.IntVar(&config.Port, "port", config.Port, "Port to surface diagnostics on")
	fs.DurationVar(&config.ShutdownGracePeriod, "shutdown-grace-period", config.ShutdownGracePeriod, "Shutdown grace period")
	fs.Parse(args[1:])

	// Limited glog config
	// See https://stackoverflow.com/questions/28207226/how-do-i-set-the-log-directory-of-glog-from-cod://stackoverflow.com/questions/28207226/how-do-i-set-the-log-directory-of-glog-from-code
	// Simulate global flags so we can configure some of the glog flags
	// Need to add global flags as the default is to exit on error - i.e. Unknown flags which is how our flags above will be seen
	fs.VisitAll(func(f *flag.Flag) { _ = flag.String((*f).Name, "", "") })
	flag.Lookup("logtostderr").Value.Set("true")
	flag.Lookup("v").Value.Set(strconv.Itoa(config.LoggingVerbosityLevel))
	flag.Parse()

	config.NamespaceBlacklistSet = deriveStringSet(config.NamespaceBlacklist)

	return config, nil
}

func getDefaultConfig() config {
	config := config{
		AuthenticationTokenRenewalInterval: defaultAuthenticationTokenRenewalInterval,
		HostNamespace:                      defaultHostNamespace,
		InformersResyncInterval:            defaultInformersResyncInterval,
		KubeConfigFilePath:                 os.Getenv("KUBECONFIG"),
		LoggingVerbosityLevel:              defaultLoggingVerbosityLevel,
		NamespaceBlacklist:                 defaultNamespaceBlacklist,
		Port:                               defaultPort,
		ShutdownGracePeriod:                defaultShutdownGracePeriod,
	}

	config.NamespaceBlacklistSet = deriveStringSet(config.NamespaceBlacklist)

	return config
}

func deriveStringSet(source string) stringset {
	set := stringset{}
	if strings.TrimSpace(source) == "" {
		return set
	}

	for _, nsName := range strings.Split(source, ",") {
		set[strings.TrimSpace(nsName)] = empty{}
	}

	return set
}
