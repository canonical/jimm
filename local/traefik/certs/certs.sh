#!/bin/sh

# A simple script to setup TLS for JIMM when running locally with compose.
# Please make sure you run this in the ./certs directory.

if [ "$1" != "--force" ]; then
    if [ -f "server.crt" ] && [ -f "server.key" ]; then
        echo "Server certs already exist. Skipping cert generation."
        echo "Run with --force to regenerate."
    fi
fi

rm -f ca.crt ca.key server.key server.crt

# Gen CA
openssl \
    req \
    -x509 \
    -nodes \
    -days 36500 \
    -newkey rsa:4096 \
    -keyout ca.key \
    -out ca.crt \
    -subj '/CN=localhost/C=UK/ST=Diglett/L=Diglett/O=Canonical'
chmod 400 ./ca.key

# Server CSR & Server key
openssl \
    req \
    -nodes \
    -new \
    -newkey rsa:4096 \
    -keyout server.key \
    -out server.csr \
    -subj '/CN=jalidy/C=UK/ST=Diglett/L=Diglett/O=JALIDY'

# Server cert
openssl \
    x509 \
    -req \
    -in server.csr \
    -days 36500 \
    -CA ca.crt \
    -CAkey ca.key \
    -CAcreateserial \
    -out server.crt \
    -extensions v3_req \
    -extfile ./san.conf
    
rm server.csr

sudo cp ca.crt /usr/local/share/ca-certificates
sudo update-ca-certificates
