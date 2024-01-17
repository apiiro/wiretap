#!/bin/bash

VERSION=$(git tag --sort=creatordate | grep -E '[0-9]' | tail -1)
AGENT_TAG=$(echo ${VERSION} | grep -o '^[0-9.]*')
echo "Version $VERSION"
echo "Tag $AGENT_TAG"

ENV_PREFIX=apiiro/network-broker
docker buildx build --platform linux/amd64 \
  --push --pull \
  -t gcr.io/$ENV_PREFIX/broker-agent:latest \
  -t gcr.io/$ENV_PREFIX/broker-agent:$AGENT_TAG \
  -f wiretap-agent.Dockerfile \
  --build-arg VERSION=${VERSION} \
  . 