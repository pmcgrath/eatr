package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// PENDING: This test is disabled for now, if i enable will set a global glog value that results in the log messages being written to stdout
// Test was only for error condition tests
func TestnewConfig(t *testing.T) {
	for _, tc := range []struct {
		Name        string
		Args        []string
		ExpectError bool
	}{
		{
			Name: "Case1",
			Args: []string{
				"-auth-token-renewal-interval", "2s",
				"-aws-credentials-secret-prefix", "aprefix-",
				"-host-namespace", "abc",
				"-informers-resync-interval", "10m",
				"-config-file-path", "/here.config",
				"-logging-verbosity-level", "0",
				"-port", "1200",
				"-shutdown-grace-period", "1H"},
			ExpectError: false,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			_, err := getConfig(tc.Args)

			assert.Equal(t, tc.ExpectError, err != nil, "Erorr")
		})
	}
}
