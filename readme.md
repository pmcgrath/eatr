# Did not bother commiting the vendor directory here
- Explicitly added as a .gitignore so repo is small



# Dependencies
- Dependency tool

```
# See https://github.com/golang/dep
go get -u github.com/golang/dep/cmd/dep

dep init
```

- Added vendor directory to .gitignore

- DANGER is i am building against HEAD which i mantain - This is not a good idea stability wise

- To build you can restore with (Will need to sort out GOPATH)
```
dep ensure
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



# App usage

```
curl localhost:5000/metrics
```
