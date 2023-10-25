packer {
  required_plugins {
    amazon = {
      source  = "github.com/hashicorp/amazon"
      version = ">= 1.0.0"
    }
  }
}

variable "aws_region" {
  type    = string
  default = "us-east-1"
}

variable "source_ami" {
  type    = string
  default = "ami-06db4d78cb1d3bbf9" # debian
}

variable "ssh_username" {
  type    = string
  default = "admin"
}

variable "subnet_id" {
  type    = string
  default = "subnet-09954eff138813c81"
}

variable "ami_name" {
  type    = string
  default = "mydefaultami"
}

variable "ami_description" {
  type    = string
  default = "AMI for WebApp"
}

# https://www.packer.io/plugins/builders/amazon/ebs
source "amazon-ebs" "my-ami-ci2" {
  region          = "${var.aws_region}"
  ami_name        = "${var.ami_name}"
  ami_description = "${var.ami_description}"
  ami_regions = [
    "us-east-1",
    "us-east-2",
  ]
  ami_users = [
    "785896633607",
  ]

  aws_polling {
    delay_seconds = 120
    max_attempts  = 50
  }

  instance_type = "t2.micro"
  source_ami    = "${var.source_ami}"
  ssh_username  = "${var.ssh_username}"
  subnet_id     = "${var.subnet_id}"

  launch_block_device_mappings {
    delete_on_termination = true
    device_name           = "/dev/xvda"
    volume_size           = 8
    volume_type           = "gp2"
  }
}

build {
  sources = ["source.amazon-ebs.my-ami-ci2"]

  provisioner "shell" {
    environment_vars = [
      "DEBIAN_FRONTEND=noninteractive",
      "CHECKPOINT_DISABLE=1"
    ]
    inline = [
      "sudo apt-get update",
      "sudo apt-get upgrade -y",
      "mkdir installations",
      "cd installations",
      "wget  https://go.dev/dl/go1.20.2.linux-amd64.tar.gz",
      "sudo tar -xvf go1.20.2.linux-amd64.tar.gz",
      "sudo mv go /usr/local",
      "export GOROOT=/usr/local/go",
      "export PATH=$GOPATH/bin:$GOROOT/bin:$PATH",
      "sudo apt-get -y install default-mysql-server",
    ]
  }

  provisioner "file" {
    source      = "/home/runner/work/webapp/webapp/target/myapp"
    destination = "/tmp/myapp"
  }

  provisioner "file" {
    source      = "/home/runner/work/webapp/webapp/config/users.csv"
    destination = "/tmp/users.csv"
  }

  provisioner "shell" {
    inline = [
      "mkdir webapp",
      "cd webapp",
      "cp /tmp/myapp .",
      "chmod +x myapp",
      "sudo cp /tmp/users.csv /opt/",
    ]
  }
   }