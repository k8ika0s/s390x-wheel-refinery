FROM rockylinux:9
RUN dnf -y update && \
    dnf -y install python3 python3-pip python3-venv gcc gcc-c++ make git openssl-devel bzip2-devel libffi-devel zlib-devel xz-devel && \
    dnf clean all
RUN python3 -m pip install --upgrade pip build
WORKDIR /app
COPY pyproject.toml README.md ./ 
COPY src ./src
RUN python3 -m pip install --no-cache-dir .
COPY containers/builder/refinery-build.sh /usr/local/bin/refinery-build
RUN chmod +x /usr/local/bin/refinery-build
WORKDIR /
ENV PATH="/usr/local/bin:/usr/bin:${PATH}"
