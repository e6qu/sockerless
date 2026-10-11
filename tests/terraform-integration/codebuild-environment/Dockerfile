# The build environment the simulator environment's AWS CodeBuild project runs
# the Lambda backend's overlay builds in. A CodeBuild project names any image
# as its environment; the curated `aws/codebuild/*-standard` images the real
# environments use are five to six gigabytes, and the buildspec needs only
# what they carry for it: the AWS CLI (to fetch the build context from Amazon
# S3 and log in to Amazon ECR), tar and gzip (to unpack the context), and the
# Docker CLI (to build and push against the engine privileged mode grants the
# build).
ARG DOCKER_CLI_VERSION=28.5.1
FROM public.ecr.aws/docker/library/docker:${DOCKER_CLI_VERSION}-cli AS docker

FROM public.ecr.aws/aws-cli/aws-cli:2.37.12
RUN dnf install -y tar gzip && dnf clean all
COPY --from=docker /usr/local/bin/docker /usr/local/bin/docker
COPY --from=docker /usr/local/libexec/docker/cli-plugins/docker-buildx /usr/local/libexec/docker/cli-plugins/docker-buildx
# The buildspec's commands run under the shell CodeBuild starts, not the
# AWS CLI the base image runs by default.
ENTRYPOINT []
CMD ["/bin/sh"]
