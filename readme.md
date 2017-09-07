# Did not bother commiting the vendor directory here
- Explicitly added as a .gitignore so repo is small



# Dependencies
- Dependency tool

```
# See https://github.com/golang/dep
go get -u github.com/golang/dep/cmd/dep

dep init -v
```

- Added vendor directory to .gitignore

- DANGER is I am building against HEAD which I mantain - This is not a good idea stability wise
	-See https://medium.com/@vladimirvivien/using-gos-dep-to-organize-your-kubernetes-client-go-dependencies-509ddc766ed3

- To build you can restore with (Will need to sort out GOPATH)
```
dep ensure -v
```

- Getting our dependencies

```
go get -u github.com/aws/aws-sdk-go/...
go get -u github.com/pkg/errors
go get -u github.com/prometheus/client_golang/prometheus/...
go get -u k8s.io/api/...
go get -u k8s.io/apimachinery/...
go get -u k8s.io/client-go/...
```

- What are the k8s.io rep deps for client-go

```
# Find all go files with a '"k8s.io' content in a line, using the " sperator get import package name, using the / separator get the second field - this is the k8s.io repo name
grep -r '"k8s.io' --include '*.go' -w ~/go/src/k8s.io/client-go | awk 'BEGIN{ FS="\"" }; { print $2 }' | awk 'BEGIN{ FS="/" }; {print $2}' | sort | uniq
```

- Issues with the deps at this time - client-golang is a moving target
	- I copied the deps based on https://github.com/heptio/ark/blob/master/Godeps/Godeps.json on commit b7265a59f2b912d733c991bd993ce75d66053d6a
	- k8s.io/api is too early to use - will check again at the end of Sept and pin the deps then (Around 1.8 release date)
	
```
dep ensure -v k8s.io/client-go@v4.0.0-beta.0
dep ensure -v k8s.io/apimachinery@abe34e4f5b4413c282a83011892cbeea5b32223b
make build
```



# App usage

```
curl localhost:5000/metrics
```
