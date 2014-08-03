FROM flynn/busybox
MAINTAINER Jonathan Rudenberg <jonathan@titanous.com>

ADD ./bin/flynn-host-docker /bin/flynn-host
ADD ./bin/manifest.json /etc/flynn-host.json

ENTRYPOINT ["/bin/flynn-host"]
