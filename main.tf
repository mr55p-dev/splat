terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "3.0.2"
    }
  }
}

provider "aws" {
  region = "eu-west-2"
}

data "aws_ecr_authorization_token" "token" {}

provider "docker" {
  ssh_opts = ["-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null"]

  registry_auth {
    address  = data.aws_ecr_authorization_token.token.proxy_endpoint
    username = data.aws_ecr_authorization_token.token.user_name
    password = data.aws_ecr_authorization_token.token.password
  }
}

data "docker_registry_image" "traefik" {
  name = "traefik:v3.0"
}

resource "docker_image" "traefik" {
  name          = data.docker_registry_image.traefik.name
  pull_triggers = [data.docker_registry_image.traefik.sha256_digest]
}

resource "docker_container" "traefik" {
  name  = "traefik"
  image = docker_image.traefik.image_id

  ports {
    internal = 80
    external = 80
  }

  ports {
    internal = 443
    external = 443
  }

  ports {
    internal = 8080
    external = 8080
  }

  volumes {
    container_path = "/var/run/docker.sock"
    host_path      = "/var/run/docker.sock"
    read_only      = true
  }

  volumes {
    container_path = "/traefik/"
    host_path      = "/home/ubuntu/traefik/"
    read_only      = false
  }

  volumes {
    container_path = "/etc/traefik/traefik.yml"
    host_path      = "/home/ubuntu/traefik.yml"
    read_only      = true
  }
}
