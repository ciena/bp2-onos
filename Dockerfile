FROM onosproject/onos:1.3
MAINTAINER Ciena - BluePlanet <blueplant@ciena.com>

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

WORKDIR /root/onos
ENTRYPOINT ["./bin/onos-service"]
