FROM        prom/busybox:latest
MAINTAINER  The alertmanager2es authors

COPY alertmanager2es /bin/alertmanager2es

EXPOSE     9097
ENTRYPOINT [ "/bin/alertmanager2es" ]
