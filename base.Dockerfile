# Cache placeholder -- NOT used as a base image (the driver Dockerfile is unchanged). This file's
# `FROM debian:bookworm-slim` + apt-get step is byte-identical to the driver Dockerfile's runtime
# stage, so CI builds this once and pushes its layers to the BuildKit GHA layer cache; the driver
# matrix builds --cache-from it and hit that (expensive) apt-get layer instead of re-running it.
# KEEP THESE STEPS IN SYNC with the runtime stage of ./Dockerfile, or the cache key won't match.
FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    openjdk-17-jre-headless \
    libxml2 \
    ca-certificates \
    libpam-modules \
    libcrypt1 \
    && rm -rf /var/lib/apt/lists/*
