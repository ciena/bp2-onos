FROM onosproject/onos:1.3
MAINTAINER Ciena - BluePlanet <blueplant@ciena.com>

RUN apt-get update && \
    apt-get install -y curl

RUN mkdir -p /bp2/save && \
    tar -P -zcf /bp2/save/clean.tgz /root/onos

WORKDIR /root/onos

ENV NBI_onos_port=8181
ENV NBI_onos_type=http
ENV NBI_onos_publish=true

ENV NBI_onosssh_port=8101
ENV NBI_onosssh_type=tcp
ENV NBI_onosshh_publish=true

ENV ONOS_APPS drivers,openflow,proxyarp,mobility,fwd

ADD bp2/hooks /bp2/hooks

RUN ln -s /bp2/hooks/onos-hook /bp2/hooks/heartbeat && \
    ln -s /bp2/hooks/onos-hook /bp2/hooks/peer-update && \
    ln -s /bp2/hooks/onos-hook /bp2/hooks/peer-status

ENV BP2HOOK_heartbeat=/bp2/hooks/heartbeat
ENV BP2HOOK_peer-update=/bp2/hooks/peer-update
ENV BP2HOOK_peer-status=/bp2/hooks/peer-status

ENTRYPOINT ["/bp2/hooks/onos-wrapper","./bin/onos-service"]
