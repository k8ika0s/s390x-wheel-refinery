FROM ubuntu:22.04
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
    apt-get install -y python3 python3-pip python3-venv build-essential git libssl-dev libbz2-dev libffi-dev zlib1g-dev xz-utils && \
    apt-get clean && rm -rf /var/lib/apt/lists/*
RUN python3 -m pip install --upgrade pip build
WORKDIR /app
COPY pyproject.toml README.md ./
COPY src ./src
RUN python3 -m pip install --no-cache-dir .
COPY containers/builder/refinery-build.sh /usr/local/bin/refinery-build
RUN chmod +x /usr/local/bin/refinery-build
WORKDIR /
ENV PATH="/usr/local/bin:/usr/bin:${PATH}"
