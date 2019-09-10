# ecs-fargate-login

A simple tool to get an ephemeral CLI in a ECS Fargate task.

```
$ go get -u github.com/keichan34/ecs-fargate-login
```

## Use cases

* Serverless [bastion host](https://docs.aws.amazon.com/quickstart/latest/linux-bastion/architecture.html).
* A way to get an application console (like [`rails console`](https://guides.rubyonrails.org/command_line.html#rails-console)) in staging / production.

## How it works

The high level outline is:

1. `ecs-fargate-login` creates a one-time use RSA key pair.
1. `ecs-fargate-login` starts a SSH server task in Fargate.
2. When the task has booted, `ecs-fargate-login` logs in to the SSH server.
3. The SSH server shuts itself down when the user logs off.

A script (`server/start-sshd.sh`) and `openssh-server` are installed in the 
Dockerfile for the SSH server. The CLI service should have its own dedicated
task definition (an example is included in `server/task-definition.json`).

When `ecs-fargate-login` boots this task definition, `start-sshd.sh` will perform
some initial setup, such as reading environment variables in to `/etc/environment`
(because SSH will strip them out otherwise), and authorizing the one-time RSA key
pair by passing the public key in the `_AUTHORIZED_PUBLIC_KEY` environment variable.

When the task has been booted, `ecs-fargate-login` will start `ssh` and the session
is created.

When the session ends, `ecs-fargate-login` deletes the one-time private key, and
the server will shut itself down.

## Requirements

In AWS:

* A security group that allows inbound SSH access (port 22) from the machine you're
  using.
* A task definition that is set up to run the SSH server.

On the server:

* `openssh-server`
* `start-sshd.sh` (see example in the `server` directory)

On the interactive client:

* `ecs-fargate-login`
* Access to AWS
  * `ecs:RunTask` for the ARN of the task definition the tool will use to boot.
  * `ecs:DescribeTasks` for all tasks
  * `ec2:DescribeNetworkInterfaces` (only supports a resource of `*`)
  * `iam:PassRole` for both the execution task role (the role AWS Fargate uses to start the task) and the task role (the role the task assumes when running)

## Quick Start

1. Create 2 ECS task IAM roles: one for the running task, and one for the task execution.
  Put the ARNs in the task definition, using `server/task-definition.json` as a template.
2. Register the task definition. The template uses `test-cli`, but you can choose any name you like.
3. Get the VPC Security Group ID (`sg-`) of a security group that allows port 22 incoming from
  the client you plan on logging in from.
4. Get the VPC Subnet ID(s) of a public subnet you want to launch this instance in to.
5. Run `ecs-fargate-login`: `ecs-fargate-login -n test-cli -sg sg-1234,sg-4321 -sn subnet-1234,subnet-4321`
