ARG binary_name=kfctl
FROM registry.access.redhat.com/ubi8/ubi-minimal:latest AS build

ENV OPERATOR=/usr/local/bin/${binary_name} \
    USER_UID=1001 \
    USER_NAME=kfctl \
    HOMEDIR=/homedir

# install operator binary
COPY _output/bin/${binary_name} ${OPERATOR}

COPY bin /usr/local/bin
RUN  /usr/local/bin/user_setup
RUN  /usr/local/bin/entrypoint

FROM alpine:3.10.1 as barebones_base
RUN apk update
RUN apk add git
ENV OPERATOR=/usr/local/bin/${binary_name} \
    USER_UID=1001 \
    USER_NAME=kfctl \
    HOMEDIR=/homedir

COPY --from=build /usr/local/bin/${binary_name} ${OPERATOR}
COPY --from=build /etc/passwd /etc/passwd
COPY --from=build ${HOMEDIR} ${HOME}

ENTRYPOINT ["${OPERATOR}"]

USER ${USER_UID}
