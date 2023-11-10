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
      "wget https://amazoncloudwatch-agent.s3.amazonaws.com/debian/amd64/latest/amazon-cloudwatch-agent.deb",
      "sudo dpkg -i -E ./amazon-cloudwatch-agent.deb",
      "sudo groupadd csye6225",
      "sudo useradd -s /bin/false -g csye6225 -d /opt/csye6225 -m csye6225",
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

  provisioner "file" {
    source      = "/home/runner/work/webapp/webapp/myapp.service"
    destination = "/tmp/myapp.service"
  }

  provisioner "file" {
    source      = "/home/runner/work/webapp/webapp/config/cloudwatch-config.json"
    destination = "/tmp/cloudwatch-config.json"
  }

  provisioner "shell" {
    inline = [
      "mkdir webapp",
      "cd webapp",
      "cp /tmp/myapp .",
      "cp /tmp/users.csv .",
      "cd ..",
      "sudo chown -R csye6225:csye6225 webapp",
      "sudo chmod 755 webapp/myapp",
    ]
  }

  provisioner "shell" {
    inline = [
      "sudo cp /tmp/myapp.service /etc/systemd/system",
      "sudo cp /tmp/cloudwatch-config.json /opt",
      "sudo systemctl daemon-reload",
      "sudo systemctl enable myapp",
      "sudo systemctl start myapp",
    ]
  }
}