FROM scratch

COPY bin/secret-injector /secret-injector

USER 10000:10000

ENTRYPOINT ["/secret-injector"]
