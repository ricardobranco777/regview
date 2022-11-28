#!/bin/bash

# Get a random port that isn't open
get_random_port () {
	while true ; do
		port=$((1024+RANDOM))
		(echo > /dev/tcp/localhost/$port) 2>/dev/null || break
	done
	echo $port
}

PATH=.:$PATH

port=$(get_random_port)
auth_port=$(get_random_port)
registry_dir=$(mktemp -d)
name=registry$port
image=regview
certs="$PWD/tests/certs"
user="testuser"
pass="testpass"

(cd $certs ; simplepki ; cat subca.pem ca.pem > cacerts.pem ; htpasswd -Bbn $user $pass > htpasswd)

cleanup () {
	set +e
	sudo docker rm -vf $name
	sudo docker rm -vf auth$auth_port
	sudo docker rmi $image
	sudo docker rmi $(sudo docker images --format '{{.Repository}}:{{.Tag}}' localhost:$port/*)
	sudo rm -rf $registry_dir
	rm -f $PWD/tests/config.json
}

trap "cleanup ; exit 1" ERR
set -xeE

sudo docker run -d \
	--net=host \
	--name $name \
	-p $port:$port \
	-e REGISTRY_HTTP_ADDR=0.0.0.0:$port \
        -e REGISTRY_STORAGE_DELETE_ENABLED=true \
	-v $registry_dir:/var/lib/registry \
	registry:2

sleep 5

# Used to test --all, --os & --arch options
sudo docker buildx create --use --driver-opt network=host
sudo docker buildx build --push --platform linux/amd64,linux/386 --tag localhost:$port/$image:latest .
sudo docker pull localhost:$port/$image:latest
sudo docker tag localhost:$port/$image:latest $image

id=$(sudo docker images --format="{{.ID}}" localhost:$port/$image:latest)

test_proto () {
	proto="$1"

	# Test listing
	$docker regview $options localhost:$port | grep -q $id
	$docker regview $options $proto://localhost:$port | grep -q $id

	# Test image
	$docker regview $options localhost:$port/$image:latest | grep -q $id
	$docker regview $options $proto://localhost:$port/$image:latest | grep -q $id

	# Test glob in repository and tag
	$docker regview $options localhost:$port/${image:0:2}* | grep -q $id
	$docker regview $options localhost:$port/${image:0:2}*:late* | grep -q $id
	$docker regview $options localhost:$port/${image:0:2}*:latest | grep -q $id
	$docker regview $options localhost:$port/$image:late* | grep -q $id
}

echo "Testing HTTP"

options="--insecure"
test_proto http

echo "Testing HTTP using Docker image"

docker="sudo docker run --rm --net=host"

test_proto http

echo "Testing multi-arch"

regview $options -a http://localhost:$port | grep -q "386$"
regview $options --arch 386 http://localhost:$port | grep -q "386$"
regview $options --arch 386 http://localhost:$port | grep -q "amd64$" && false

echo "Testing --delete"

echo testing > testing
cat > /tmp/Dockerfile << EOF
FROM scratch
COPY testing .
EOF

sudo docker build -t localhost:$port/testing -f /tmp/Dockerfile .
rm -f testing
sudo docker push localhost:$port/testing
regview $options --delete --dry-run http://localhost:$port/testing:latest
regview $options --delete --verbose http://localhost:$port/testing:latest
sudo docker restart $name
sleep 5
regview $options http://localhost:$port | grep -q testing && false

unset docker docker_options

sudo docker rm -vf $name

sudo docker run -d \
	--net=host \
	--name $name \
	-p $port:$port \
	-e REGISTRY_HTTP_ADDR=0.0.0.0:$port \
	-e REGISTRY_AUTH=htpasswd \
	-e REGISTRY_AUTH_HTPASSWD_REALM=xxx \
	-e REGISTRY_AUTH_HTPASSWD_PATH=/certs/htpasswd \
       	-e REGISTRY_HTTP_TLS_CERTIFICATE=/certs/server.pem \
       	-e REGISTRY_HTTP_TLS_KEY=/certs/server.key \
       	-e REGISTRY_HTTP_TLS_CLIENTCAS=" - /certs/cacerts.pem" \
        -e REGISTRY_STORAGE_DELETE_ENABLED=true \
	-v $registry_dir:/var/lib/registry \
	-v $certs:/certs:ro \
	registry:2

sleep 5

echo "Testing HTTPS with Basic Auth with username & password specified"

options="-c $certs/client.pem -k $certs/client.key -C $certs/cacerts.pem"
options="$options -u $user -p $pass"

test_proto https

echo "Testing HTTPS with Basic Auth getting credentials from config.json"

export DOCKER_CONFIG="$PWD/tests"
cat > $DOCKER_CONFIG/config.json <<- EOF
	{"auths": {"https://localhost:$port": {"auth": "$(echo -n $user:$pass | base64)"}}}
EOF
options="-c $certs/client.pem -k $certs/client.key -C $certs/cacerts.pem"
test_proto https
unset DOCKER_CONFIG

echo "Testing HTTPS with Basic Auth with username & password specified using Docker image"

docker="sudo docker run --rm --net=host -v $certs:/certs:ro"
options="-c /certs/client.pem -k /certs/client.key -C /certs/cacerts.pem -u $user -p $pass"
test_proto https

echo "Testing HTTPS with Token Auth with username & password specified"

sudo docker rm -vf $name

sed -i "s/addr:.*/addr: \":$auth_port\"/" tests/config/auth_config.yml

sudo docker run -d \
        --net=host \
        --name auth$auth_port \
        -p $auth_port:$auth_port \
        -v $certs:/ssl:ro \
        -v $PWD/tests/config:/config:ro \
        cesanta/docker_auth \
        /config/auth_config.yml

sudo docker run -d \
        --net=host \
        --name $name \
        -p $port:$port \
        -e REGISTRY_HTTP_ADDR=0.0.0.0:$port \
        -e REGISTRY_AUTH=token \
        -e REGISTRY_HTTP_TLS_CERTIFICATE=/certs/server.pem \
        -e REGISTRY_HTTP_TLS_KEY=/certs/server.key \
        -e REGISTRY_AUTH_TOKEN_REALM=https://localhost:$auth_port/auth \
        -e REGISTRY_AUTH_TOKEN_SERVICE="Docker registry" \
        -e REGISTRY_AUTH_TOKEN_ISSUER="Auth Service" \
        -e REGISTRY_AUTH_TOKEN_ROOTCERTBUNDLE=/certs/server.pem \
        -v $registry_dir:/var/lib/registry \
        -v $certs:/certs:ro \
        registry:2

options="-C /certs/cacerts.pem -u admin -p badmin --debug -v"
docker="sudo docker run --rm --net=host -v $certs:/certs:ro"

test_proto https

cleanup
