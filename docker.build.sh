#!/bin/bash

# ENV_PREFIX=apiiro-rnd-network-broker
ENV_PREFIX=apiiro/network-broker
AGENT_TAG=0.7
docker buildx build --platform linux/amd64 \
  --push --pull \
  -t gcr.io/$ENV_PREFIX/broker-agent:latest \
  -t gcr.io/$ENV_PREFIX/broker-agent:$AGENT_TAG \
  -f wiretap-agent.Dockerfile \
  . 