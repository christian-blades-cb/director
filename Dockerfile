FROM scratch

MAINTAINER Christian Blades <christian.blades@gmail.com>

ADD director /
EXPOSE 8000 8888
ENTRYPOINT [ "/director" ]
