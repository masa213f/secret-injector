
```
# In root dir
$ make image-build
$ kind create cluster
$ kind load docker-image masa213f/secret-injector:0.1.0
```


```
# In example dir
$ make certs
$ kubectl create namespace secret-injector
$ kubectl apply -k .
$ kubectl apply -f secret.yaml
```
