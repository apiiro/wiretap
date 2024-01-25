#!/bin/sh

WIRETAP_RELAY_INTERFACE_IPV4=$TUNNEL_SUBNET.2 \
WIRETAP_RELAY_PEER_ALLOWED=$TUNNEL_SUBNET.1/32 \
WIRETAP_RELAY_INTERFACE_PRIVATEKEY=$AGENT_PRIVATE_KEY \
WIRETAP_RELAY_INTERFACE_MTU=$RELAY_MTU \
WIRETAP_RELAY_PEER_PUBLICKEY=$APIIRO_PUBLIC_KEY \
WIRETAP_RELAY_PEER_ENDPOINT=$APIIRO_ENDPOINT \
WIRETAP_SIMPLE=true \
WIRETAP_MAPPING_PREFIX=$MAPPING_PREFIX \
WIRETAP_MAPPING_HOSTS=$MAPPING_HOSTS \
WIRETAP_CONFIG_TOKEN=$CONFIG_TOKEN \
WIRETAP_APIIRO_DOMAIN=$APIIRO_DOMAIN \
WIRETAP_SKIP_SSL_VERIFY=$SKIP_SSL_VERIFY \
WIRETAP_VERBOSE=$VERBOSE_LOGS \
exec ./wiretap serve "$@"