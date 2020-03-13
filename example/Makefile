BUILD_DIR=./certs/build

CA_FILES=$(addprefix $(BUILD_DIR)/, ca.csr ca.pem ca-key.pem)
SERVER_CERT_FILES=$(addprefix $(BUILD_DIR)/, server.csr server.pem server-key.pem)

all: certs

setup:
	go install github.com/cloudflare/cfssl/cmd/cfssl
	go install github.com/cloudflare/cfssl/cmd/cfssljson

certs: $(SERVER_CERT_FILES)

clean:
	-rm $(BUILD_DIR)/*

$(CA_FILES): ./certs/csr.json
	mkdir -p build/certs
	cfssl gencert -initca certs/csr.json | cfssljson -bare $(BUILD_DIR)/ca

$(SERVER_CERT_FILES): $(CA_FILES) ./certs/ca-config.json ./certs/server.json
	cfssl gencert -ca=$(BUILD_DIR)/ca.pem -ca-key=$(BUILD_DIR)/ca-key.pem -config=certs/ca-config.json -profile=server certs/server.json | cfssljson -bare $(BUILD_DIR)/server

.PHONY: all setup certs clean
