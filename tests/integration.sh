#!/bin/bash

# Get a random port that isn't open
get_random_port () {
	while true ; do
		port=$((1024+RANDOM))
		(echo > /dev/tcp/localhost/$port) 2>/dev/null || break
	done
	echo $port
}

port=$(get_random_port)
name=registry$port
image=regview
certs="$PWD/tests/certs"
user="testuser"
pass="testpass"

(cd $certs ; simplepki ; htpasswd -Bbn $user $pass > htpasswd)

cleanup () {
	set +e
	sudo docker rm -vf $name
	sudo docker rmi $image
	sudo docker rmi $(sudo docker images --format '{{.Repository}}:{{.Tag}}' localhost:$port/*)
	rm -f $PWD/tests/config.json
	rm -f $certs/*
}

#trap "cleanup ; exit 1" ERR
set -xeE

python_version=$(python3 --version | cut -d. -f2)
sed -i "s/3\.9/3.$python_version/" Dockerfile

sudo docker build -t $image .

sudo docker run -d \
	--net=host \
	--name $name \
	-p $port:$port \
	-e REGISTRY_HTTP_ADDR=0.0.0.0:$port \
	-v /tmp/registry:/var/lib/registry \
	registry:2

sleep 5

sudo docker tag $image localhost:$port/$image:latest
sudo docker push localhost:$port/$image:latest

id=$(sudo docker images --no-trunc --format="{{.ID}}" localhost:$port/$image:latest)
digest=$(sudo docker images --digests --format="{{.Digest}}" localhost:$port/$image)

test_proto () {
	proto="$1"

	# Test listing
	$docker regview $options --digests localhost:$port | grep -q $digest
	$docker regview $options --digests $proto://localhost:$port | grep -q $digest

	# Test image
	$docker regview $options localhost:$port/$image:latest | grep -q $digest
	$docker regview $options $proto://localhost:$port/$image:latest | grep -q $digest

	# Test digest
	$docker regview $options localhost:$port/$image@$digest | grep -q $digest
	$docker regview $options $proto://localhost:$port/$image@$digest | grep -q $digest

	# Test glob in repository and tag
	$docker regview $options --digests localhost:$port/${image:0:2}* | grep -q $digest
	$docker regview $options --digests localhost:$port/${image:0:2}*:late* | grep -q $digest
	$docker regview $options --digests localhost:$port/${image:0:2}*:latest | grep -q $digest
	$docker regview $options --digests localhost:$port/$image:late* | grep -q $digest
}

echo "Testing HTTP"

test_proto http

echo "Testing HTTP using Docker image"

docker="sudo docker run --rm --net=host"

test_proto http

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
	-v /tmp/registry:/var/lib/registry \
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

echo "Testing --insecure option"
docker="sudo docker run --rm --net=host -v $certs:/certs:ro"
options="-c /certs/client.pem -k /certs/client.key --insecure -u $user -p $pass"
$docker regview $options $proto://localhost:$port/$image:latest | grep -q $digest

echo "Testing --verbose"
docker="sudo docker run --rm --net=host -v $certs:/certs:ro"
options="-c /certs/client.pem -k /certs/client.key --insecure -v -u $user -p $pass"
$docker regview $options $proto://localhost:$port/$image:latest | grep History | grep ENTRYPOINT | grep -q regview

echo "Testing HTTPS with Token Auth with username & password specified"

sudo docker rm -vf $name

auth_port=$(get_random_port)

sed -i "s/5001/$auth_port/g" tests/config/auth_config.yml

sudo docker run -d \
	--net=host \
	--name auth \
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
	-v /tmp/registry:/var/lib/registry \
	-v $certs:/certs:ro \
	registry:2

options="-C /certs/cacerts.pem -u admin -p badmin --debug -v"
docker="sudo docker run --rm --net=host -v $certs:/certs:ro"

test_proto https

# Test token cache
test $($docker regview $options https://localhost:$port 2>/dev/null | grep -c " 401 ") -eq 1
test $($docker regview $options https://localhost:$port/$image:latest 2>/dev/null | grep -c " 401 ") -eq 1

sudo docker rm -vf $name auth

echo "Testing pagination"

sudo docker run -d \
        --net=host \
        --name $name \
        -p $port:$port \
        -e REGISTRY_HTTP_ADDR=0.0.0.0:$port \
        -v /tmp/registry:/var/lib/registry \
        registry:2
sleep 5

sed -ri 's/(_catalog|\/tags\/list)/\1?n=1/' _regview/docker_registry.py

for i in ${image}:test ${image}2:latest ${image}2:test ; do
	sudo docker tag $image localhost:$port/$i
	sudo docker push localhost:$port/$i
done

test $(regview -v http://localhost:$port | wc -l) -eq 5

cleanup
