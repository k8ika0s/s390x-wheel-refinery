FROM fedora:40
RUN dnf -y update && \
    dnf -y install python3 python3-pip python3-venv gcc gcc-c++ make git openssl-devel bzip2-devel libffi-devel zlib-devel xz-devel && \
    dnf clean all
RUN python3 -m pip install --upgrade pip build
ENV PATH="/usr/local/bin:/usr/bin:${PATH}"
