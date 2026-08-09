#!/bin/bash
# Generate a CA, sub-CA, server cert and client cert for e2e testing.
# Drop-in replacement for the "simplepki" binary: produces the same
# ca/subca/server/client .pem + .key files in the current directory.
set -eu

days=397 # ~ one year, one month, one day

subj() {
	printf '/O=simplepki/OU=root@localhost/CN=%s' "$1"
}

# gen_ca NAME ISSUER_KEY ISSUER_CERT PATHLEN
gen_ca() {
	name=$1 issuer_key=$2 issuer_cert=$3 pathlen=$4

	openssl ecparam -name prime256v1 -genkey -noout -out "$name.key"

	if [ -z "$issuer_key" ]; then
		openssl req -x509 -new -key "$name.key" -sha256 -days "$days" \
			-subj "$(subj "$name")" \
			-addext "basicConstraints=critical,CA:true,pathlen:$pathlen" \
			-addext "keyUsage=critical,keyCertSign,cRLSign,digitalSignature" \
			-out "$name.pem"
	else
		openssl req -new -key "$name.key" -subj "$(subj "$name")" -out "$name.csr"
		openssl x509 -req -in "$name.csr" -CA "$issuer_cert" -CAkey "$issuer_key" \
			-CAcreateserial -days "$days" -sha256 \
			-extfile <(printf 'basicConstraints=critical,CA:true,pathlen:%s\nkeyUsage=critical,keyCertSign,cRLSign,digitalSignature\n' "$pathlen") \
			-out "$name.pem"
		rm -f "$name.csr"
	fi
}

# gen_leaf NAME ISSUER_KEY ISSUER_CERT EXTENDEDKEYUSAGE
gen_leaf() {
	name=$1 issuer_key=$2 issuer_cert=$3 eku=$4

	openssl ecparam -name prime256v1 -genkey -noout -out "$name.key"
	openssl req -new -key "$name.key" -subj "$(subj "$name")" -out "$name.csr"
	openssl x509 -req -in "$name.csr" -CA "$issuer_cert" -CAkey "$issuer_key" \
		-CAcreateserial -days "$days" -sha256 \
		-extfile <(printf 'basicConstraints=critical,CA:false\nkeyUsage=critical,digitalSignature,keyEncipherment\nextendedKeyUsage=%s\nsubjectAltName=DNS:localhost,IP:127.0.0.1,IP:::1\n' "$eku") \
		-out "$name.pem"
	rm -f "$name.csr"
}

gen_ca ca "" "" 1
gen_ca subca ca.key ca.pem 0
gen_leaf client subca.key subca.pem clientAuth,serverAuth
gen_leaf server subca.key subca.pem serverAuth

rm -f ca.srl subca.srl
