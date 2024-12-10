#!/bin/bash

VERSION=$(git tag --sort=creatordate | grep -E '[0-9]' | tail -1)
AGENT_TAG=$(echo ${VERSION} | grep -o '^[0-9.]*')
echo "Version $VERSION"
echo "Tag $AGENT_TAG"

ENV_PREFIX=apiiro/public-images/network-broker
docker buildx build --platform linux/amd64 \
  --push --pull \
  -t us-docker.pkg.dev/$ENV_PREFIX/broker-agent:latest \
  -t us-docker.pkg.dev/$ENV_PREFIX/broker-agent:$AGENT_TAG \
  -f wiretap-agent.Dockerfile \
  --build-arg VERSION=${VERSION} \
  . 