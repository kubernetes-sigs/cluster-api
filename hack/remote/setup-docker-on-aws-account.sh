#!/bin/bash

# Copyright 2023 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

REPO_ROOT=$(git rev-parse --show-toplevel)
cd "${REPO_ROOT}" || exit 1

# shellcheck source=./hack/remote/setup-docker-lib.sh
source "${REPO_ROOT}/hack/remote/setup-docker-lib.sh"

#######################################################
echo -e "# Server Configuration\n"
#######################################################
# SERVER_NAME is the name of the server.
SERVER_NAME=${SERVER_NAME:-"capi-docker"}
# SERVER_PRIVATE_IP is the private IP of the server.
# Note: Must be inside of AWS_NETWORK_CIDR.
# Note: We will be able to access this IP directly because we will open a
# tunnel to AWS_NETWORK_CIDR with sshuttle.
SERVER_PRIVATE_IP=${SERVER_PRIVATE_IP:-"10.0.3.15"}
echo -e "  SERVER_NAME: ${SERVER_NAME}\n  SERVER_PRIVATE_IP: ${SERVER_PRIVATE_IP}"
# SERVER_KIND_SUBNET is the subnet of the kind network on the server.
# Note: We will be able to access this network directly because we will open a
# tunnel to SERVER_KIND_SUBNET with sshuttle. This includes all container running
# in this network.
SERVER_KIND_SUBNET=${SERVER_KIND_SUBNET:-"172.24.0.0/16"}
# SERVER_KIND_GATEWAY is the gateway of the kind network on the server.
SERVER_KIND_GATEWAY=${SERVER_KIND_GATEWAY:-"172.24.0.1"}
echo -e "  SERVER_KIND_SUBNET: ${SERVER_KIND_SUBNET}\n  SERVER_KIND_GATEWAY: ${SERVER_KIND_GATEWAY}"
echo ""
#######################################################

#######################################################
echo -e "# AWS Configuration\n"
#######################################################
# AWS_REGION is the AWS region.
# FIXME(sbueringer): cleanup ap-southeast region, zone, ..
AWS_REGION=${AWS_REGION:-"eu-central-1"}
#AWS_REGION=${AWS_REGION:-"ap-southeast-1"}
# AWS_ZONE is the AWS zone.
AWS_ZONE=${AWS_ZONE:-"eu-central-1a"}
#AWS_ZONE=${AWS_ZONE:-"ap-southeast-1a"}
# AWS_NETWORK_NAME is the name of the VPC and all the network
# objects we create in the VPC.
AWS_NETWORK_NAME=${AWS_NETWORK_NAME:-"${SERVER_NAME}"}
# AWS_NETWORK_CIDR is the CIDR of the AWS network.
# Note: The server will be part of this network.
# Note: We will be able to access this network directly because we will open a
# tunnel to AWS_NETWORK_CIDR with sshuttle.
AWS_NETWORK_CIDR=${AWS_NETWORK_CIDR:-"10.0.3.0/24"}
echo -e "  AWS_REGION: ${AWS_REGION}\n  AWS_ZONE: ${AWS_ZONE}\n  AWS_NETWORK_NAME: ${AWS_NETWORK_NAME}\n  AWS_NETWORK_CIDR: ${AWS_NETWORK_CIDR}"
# AWS_MACHINE_TYPE is the machine type for the server.
# Choose via: https://eu-central-1.console.aws.amazon.com/ec2/v2/home?region=eu-central-1#InstanceTypes
# For example:
# * c5.4xlarge  16 vCPU 32 GB RAM => ~ 0.776 USD per hour
# * c5.12xlarge 48 vCPU 96 GB RAM => ~ 2.328 USD per hour
AWS_MACHINE_TYPE=${AWS_MACHINE_TYPE:-"c5.4xlarge"}
# AWS_AMI is the AMI we will use for the server.
# AMIs:
# * Canonical, Ubuntu, 22.04 LTS, amd64 jammy image build on 2023-02-08 id:
#   * eu-central-1:   ami-0d1ddd83282187d18
#   * ap-southeast-1: ami-082b1f4237bd816a1
# FIXME(sbueringer)
AWS_AMI=${AWS_AMI:-"ami-0d1ddd83282187d18"}
#AWS_AMI=${AWS_AMI:-"ami-082b1f4237bd816a1"}
echo -e "  AWS_MACHINE_TYPE: ${AWS_MACHINE_TYPE}\n  AWS_AMI: ${AWS_AMI}"
# AWS_KEY_PAIR is the key pair we use to access the server.
# Prepare key pair with:
# # Create key pair:
# aws ec2 create-key-pair --key-name capi-docker --query 'KeyMaterial' --region "${AWS_REGION}" --output text > ${AWS_KEY_PAIR_PRIVATE_KEY_FILE}
# chmod 0400 ${AWS_KEY_PAIR_PRIVATE_KEY_FILE}
# # Add to key to local ssh agent and generate public key:
# ssh-add ${AWS_KEY_PAIR_PRIVATE_KEY_FILE}
# ssh-keygen -y -f ${AWS_KEY_PAIR_PRIVATE_KEY_FILE} > ${AWS_KEY_PAIR_PUBLIC_KEY_FILE}
AWS_KEY_PAIR=${AWS_KEY_PAIR:-"capi-docker"}
# AWS_KEY_PAIR_PUBLIC_KEY_FILE is the public key file.
# Note: This key file will be added to authorized keys of the cloud user on the server.
AWS_KEY_PAIR_PUBLIC_KEY_FILE=${AWS_KEY_PAIR_PUBLIC_KEY_FILE:-"${HOME}/.ssh/aws-capi-docker.pub"}
# AWS_KEY_PAIR_PUBLIC_KEY_FILE is the private key file.
# Note: This key file will be used in the ssh cmd to access the server.
AWS_KEY_PAIR_PRIVATE_KEY_FILE=${AWS_KEY_PAIR_PRIVATE_KEY_FILE:-"${HOME}/.ssh/aws-capi-docker"}
# AWS_SSH_CMD is the ssh cmd we use to access the server.
AWS_SSH_CMD=$(get_ssh_cmd "${AWS_KEY_PAIR_PRIVATE_KEY_FILE}" "${AWS_KEY_PAIR_PUBLIC_KEY_FILE}")
echo -e "  AWS_KEY_PAIR: ${AWS_KEY_PAIR}\n  AWS_KEY_PAIR_PUBLIC_KEY_FILE: ${AWS_KEY_PAIR_PUBLIC_KEY_FILE}\n  AWS_KEY_PAIR_PRIVATE_KEY_FILE: ${AWS_KEY_PAIR_PRIVATE_KEY_FILE}"
echo ""
# Disable pagination of AWS CLI.
export AWS_PAGER=""
#######################################################

# init_infrastructure creates the basic infrastructure:
# * VPC, Subnet, Security Group, Internet gateway and Routes
# Note: This also allows ingress traffic on port 22 for ssh access (via the security group).
# Example: create_infrastructure
function create_infrastructure() {
  echo -e "# Create Infrastructure \n"

  if [[ ${AWS_NETWORK_NAME} != "default" ]]; then
    if [[ $(aws ec2 describe-vpcs --filters Name=tag:Name,Values="${AWS_NETWORK_NAME}" --region="${AWS_REGION}" --query 'length(*[0])') = "0" ]];
    then
      # Create VPC.
      echo "Create VPC with name ${AWS_NETWORK_NAME}"
      aws ec2 create-vpc --cidr-block "${AWS_NETWORK_CIDR}" --tag-specifications "ResourceType=vpc,Tags=[{Key=Name,Value=${AWS_NETWORK_NAME}}]" --region="${AWS_REGION}"
      # Get VPC ID.
      local aws_vpc_id
      aws_vpc_id=$(aws ec2 describe-vpcs --filters Name=tag:Name,Values="${AWS_NETWORK_NAME}" --region "${AWS_REGION}" --query '*[0].VpcId' --output text)

      # Create subnet.
      echo "Create subnet with name ${AWS_NETWORK_NAME}"
      aws ec2 create-subnet --cidr-block "${AWS_NETWORK_CIDR}" --vpc-id "${aws_vpc_id}" --tag-specifications "ResourceType=subnet,Tags=[{Key=Name,Value=${AWS_NETWORK_NAME}}]" --region "${AWS_REGION}" --availability-zone "${AWS_ZONE}"
      # Get route table ID.
      local aws_route_table_id
      aws_route_table_id=$(aws ec2 describe-route-tables --filters "Name=vpc-id,Values=${aws_vpc_id}" --region "${AWS_REGION}" --query '*[0].RouteTableId' --output text)

      # Create security group.
      echo "Create security group with name ${AWS_NETWORK_NAME}"
      aws ec2 create-security-group --group-name "${AWS_NETWORK_NAME}" --description "${AWS_NETWORK_NAME}" --vpc-id "${aws_vpc_id}" --tag-specifications "ResourceType=security-group,Tags=[{Key=Name,Value=${AWS_NETWORK_NAME}}]" --region="${AWS_REGION}"
      # Get security group ID.
      local aws_security_group_id
      aws_security_group_id=$(aws ec2 describe-security-groups --filters Name=tag:Name,Values="${AWS_NETWORK_NAME}" --region "${AWS_REGION}" --query '*[0].GroupId' --output text)
      # Allow port 22 for ssh.
      echo "Allow ingress on port 22 on security group with name ${AWS_NETWORK_NAME}"
      aws ec2 authorize-security-group-ingress --group-id "${aws_security_group_id}" --protocol tcp --port 22 --cidr 0.0.0.0/0 --region="${AWS_REGION}"

      # Create internet gateway.
      # Documentation to enable internet access for subnet:
      # https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/TroubleshootingInstancesConnecting.html#TroubleshootingInstancesConnectionTimeout
      echo "Create internet gateway with name ${AWS_NETWORK_NAME}"
      aws ec2 create-internet-gateway --tag-specifications "ResourceType=internet-gateway,Tags=[{Key=Name,Value=${AWS_NETWORK_NAME}}]" --region="${AWS_REGION}"
      # Get internet gateway ID.
      local aws_internet_gateway_id
      aws_internet_gateway_id=$(aws ec2 describe-internet-gateways --filters Name=tag:Name,Values="${AWS_NETWORK_NAME}" --region "${AWS_REGION}" --query '*[0].InternetGatewayId' --output text)
      # Attach internet gateway to VPC.
      echo "Attach internet gateway with name ${AWS_NETWORK_NAME} to VPC"
      aws ec2 attach-internet-gateway --internet-gateway-id "${aws_internet_gateway_id}" --vpc-id "${aws_vpc_id}" --region="${AWS_REGION}"
      # Create routes for internet egress traffic.
      echo "Create routes for IPv4 and IPv6 egress internet traffic"
      aws ec2 create-route --route-table-id "${aws_route_table_id}" --destination-cidr-block 0.0.0.0/0 --gateway-id "${aws_internet_gateway_id}" --region "${AWS_REGION}"
      aws ec2 create-route --route-table-id "${aws_route_table_id}" --destination-ipv6-cidr-block ::/0 --gateway-id "${aws_internet_gateway_id}" --region "${AWS_REGION}"
    else
      echo "There is already a VPC with name ${AWS_NETWORK_NAME}. Skipping creation of VPC and corresponding objects."
    fi
  else
    echo "Nothing to do for default VPC."
  fi

  echo ""
}

# create_server creates a server with a Docker engine.
# Example: create_server $server_name $server_private_ip
function create_server {
  echo -e "# Create Server \n"

  local server_name=$1 && shift
  local server_private_ip=$1 && shift

  # Template user data for cloud-init.
  local userdata_file
  userdata_file=$(template_cloud_init_file "${server_private_ip}" "${AWS_KEY_PAIR_PUBLIC_KEY_FILE}")

  # Create the server if there is no running server with the same name.
  if [[ $(aws ec2 describe-instances --filters Name=tag:Name,Values="${server_name}" --filters Name=instance-state-name,Values=running --region="${AWS_REGION}" --query 'length(*[0])') = "0" ]];
  then
    local aws_subnet_id
    aws_subnet_id=$(aws ec2 describe-subnets --filters Name=tag:Name,Values="${AWS_NETWORK_NAME}" --region "${AWS_REGION}" --query '*[0].SubnetId' --output text)
    local aws_security_group_id
    aws_security_group_id=$(aws ec2 describe-security-groups --filters Name=tag:Name,Values="${AWS_NETWORK_NAME}" --region "${AWS_REGION}" --query '*[0].GroupId' --output text)

    echo "Create server with name ${server_name}"
    # Note: /dev/sda1 is renamed to /dev/nvme0n1 by AWS
    aws ec2 run-instances --tag-specifications "ResourceType=instance,Tags=[{Key=Name,Value=${server_name}}]" \
      --region "${AWS_REGION}" \
      --placement "AvailabilityZone=${AWS_ZONE}" \
      --image-id "${AWS_AMI}" \
      --instance-type "${AWS_MACHINE_TYPE}" \
      --block-device-mappings 'DeviceName=/dev/sda1,Ebs={VolumeSize=100}' \
      --subnet-id "${aws_subnet_id}"  \
      --private-ip-address "${server_private_ip}" \
      --count 1 \
      --associate-public-ip-address \
      --security-group-ids "${aws_security_group_id}"  \
      --key-name "${AWS_KEY_PAIR}" \
      --user-data "file://${userdata_file}" \
      --no-paginate

    echo "Wait until server has a public IP."
    # shellcheck disable=SC2046
    retry 3 10 [ ! -z $(aws ec2 describe-instances \
       --filters "Name=tag:Name,Values=${server_name}" \
       --region "${AWS_REGION}" \
       --query 'Reservations[*].Instances[*].PublicIpAddress' \
       --output text) ]
  else
    echo "There is already a running server with name ${server_name}. Skipping server creation."
  fi

  echo ""
}

# cleanup stops sshuttle and exits.
# Example: cleanup
function cleanup {
  stop_sshuttle
  exit 0
}

function main() {
  if [ "${1:-}" == "cleanup" ]; then
        cleanup
  fi

  if [[ -n "${SKIP_INIT_INFRA:-}" ]]; then
    echo "Skipping infrastructure initialization."
  else
    create_infrastructure
  fi

  # Create server with a Docker engine.
  create_server "${SERVER_NAME}" "${SERVER_PRIVATE_IP}"
  server_public_ip=$(aws ec2 describe-instances \
                           --filters "Name=tag:Name,Values=${SERVER_NAME}" \
                           --region "${AWS_REGION}" \
                           --query 'Reservations[*].Instances[*].PublicIpAddress' \
                           --output text)

  echo -e "# Server running: public ip: ${server_public_ip}, private ip: ${SERVER_PRIVATE_IP}\n"

  # Open the tunnel.
  start_sshuttle "${server_public_ip}" "${AWS_NETWORK_CIDR}" "${SERVER_KIND_SUBNET}" "${AWS_SSH_CMD}"

  # Wait for cloud-init to complete.
  # Note: As we already opened the tunnel we can access the server with its private ip.
  wait_for_cloud_init "${SERVER_PRIVATE_IP}" "${AWS_SSH_CMD}"

  # Wait until the docker engine is available.
  echo -e "Wait until Docker is available\n"
  export DOCKER_HOST=tcp://10.0.3.15:2375
  retry 5 30 "docker version"
  echo ""

  echo "Docker now available. Set DOCKER_HOST=${DOCKER_HOST} to use it with the Docker CLI."
}

main "$@"
