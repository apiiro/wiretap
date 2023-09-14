#!/bin/bash

# ENV_PREFIX=apiiro-rnd-network-broker
  # -t gcr.io/$ENV_PREFIX/broker-agent:latest \
  # -t gcr.io/$ENV_PREFIX/broker-agent:$AGENT_TAG \
ENV_PREFIX=apiiro/network-broker
AGENT_TAG=0.4.6
docker buildx build --platform linux/amd64 \
  --push --pull \
  -t apiiro.jfrog.io/vladimir-vuln-tests/broker-agent:0.5.0 \
  -f wiretap-agent.Dockerfile \
  . 