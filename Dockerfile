FROM quay.io/cybozu/ubuntu:18.04

COPY bin/secret-injector /secret-injector

USER 10000:10000

ENTRYPOINT ["/secret-injector"]
