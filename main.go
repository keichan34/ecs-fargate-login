/*
Copyright © 2019 Keitaroh Kobayashi

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/keichan34/ecs-fargate-login/utils"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
)

func main() {
	var taskDefinitionName, clusterName, cliContainerName, securityGroupsStr, subnetsStr string
	var assignPublicIP bool

	flag.StringVar(&taskDefinitionName, "n", "", "[Required] The name of the task definition this script will run an instance of.")
	flag.StringVar(&securityGroupsStr, "sg", "", "[Required] A comma-delimited list of security groups the booted task should be assigned.")
	flag.StringVar(&subnetsStr, "sn", "", "[Required] A comma-delimited list of subnets the task should consider when booting.")

	flag.BoolVar(&assignPublicIP, "public", true, "Whether the ECS task should be assigned a public IP or not. Default `true`.")
	flag.StringVar(&clusterName, "cluster", "default", "The name of the ECS cluster. Default `default`.")
	flag.StringVar(&cliContainerName, "cli-container-name", "cli", "The name of the container in the task definition that refers to the SSH server. Default `cli`")
	flag.Parse()

	if taskDefinitionName == "" || securityGroupsStr == "" || subnetsStr == "" {
		flag.Usage()
		os.Exit(1)
		return
	}

	keyPair := generateKeyPair()

	sess := session.Must(session.NewSession())

	taskArn := runTask(sess, taskDefinitionName, clusterName, keyPair, assignPublicIP, securityGroupsStr, subnetsStr)

	fmt.Printf("Started task: %s\n", *taskArn)

	taskIP := getTaskIPAddress(sess, clusterName, taskArn, cliContainerName, assignPublicIP)
	if taskIP == nil {
		panic(fmt.Errorf("Couldn't determine task IP address"))
	}

	fmt.Printf("Task %s is running at %s!\n", *taskArn, *taskIP)

	tmpfile, err := ioutil.TempFile("", "tmpsshkey")
	if err != nil {
		fmt.Println("We weren't able to save the key.")
		panic("Error; exiting.")
	}
	defer os.Remove(tmpfile.Name())
	if _, err := tmpfile.Write([]byte(keyPair.PrivateKeyPEM)); err != nil {
		fmt.Println("We weren't able to save the key.")
		panic("Error; exiting.")
	}

	cmd := exec.Command(
		"ssh",
		"-p",
		"22", // <- TODO: support different SSH port
		"-i",
		tmpfile.Name(),
		"-o",
		"UserKnownHostsFile=/dev/null",
		"-o",
		"StrictHostKeyChecking=no",
		fmt.Sprintf("root@%s", *taskIP), // <- TODO: support different user
	)
	// redirect the output to terminal
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.Run()
}

func generateKeyPair() *utils.SSHKeyPair {
	keyPair, err := utils.GenerateSSHKeyPair()
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	return keyPair
}

func runTask(sess *session.Session, taskDefinitionName string, clusterName string, keyPair *utils.SSHKeyPair, assignPublicIP bool, securityGroupsStr string, subnetsStr string) *string {
	ecsSvc := ecs.New(sess)

	var assignPublicIPStr string
	if assignPublicIP == true {
		assignPublicIPStr = "ENABLED"
	} else {
		assignPublicIPStr = "DISABLED"
	}

	resp, err := ecsSvc.RunTask(&ecs.RunTaskInput{
		TaskDefinition: aws.String(taskDefinitionName),
		Cluster:        aws.String(clusterName),
		StartedBy:      aws.String("ecs-fargate-login"),
		Overrides: &ecs.TaskOverride{
			ContainerOverrides: []*ecs.ContainerOverride{
				&ecs.ContainerOverride{
					Name: aws.String("cli"),
					Environment: []*ecs.KeyValuePair{
						&ecs.KeyValuePair{
							Name:  aws.String("_AUTHORIZED_PUBLIC_KEY"),
							Value: aws.String(keyPair.PublicKeyAuthorizedKey),
						},
					},
				},
			},
		},
		LaunchType: aws.String("FARGATE"),
		NetworkConfiguration: &ecs.NetworkConfiguration{
			AwsvpcConfiguration: &ecs.AwsVpcConfiguration{
				AssignPublicIp: aws.String(assignPublicIPStr),
				SecurityGroups: aws.StringSlice(strings.Split(securityGroupsStr, ",")),
				Subnets:        aws.StringSlice(strings.Split(subnetsStr, ",")),
			},
		},
	})
	ensureNoEcsError(err)

	task := resp.Tasks[0]
	return task.TaskArn
}

func getTaskIPAddress(sess *session.Session, clusterName string, taskArn *string, cliContainerName string, publicIP bool) *string {
	ecsSvc := ecs.New(sess)

	describeTaskInput := &ecs.DescribeTasksInput{
		Cluster: aws.String(clusterName),
		Tasks: []*string{
			taskArn,
		},
	}
	err := ecsSvc.WaitUntilTasksRunning(describeTaskInput)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == request.WaiterResourceNotReadyErrorCode {
			ecsTaskError(ecsSvc, describeTaskInput)
		}
	}
	ensureNoEcsError(err)

	resp, err := ecsSvc.DescribeTasks(describeTaskInput)
	ensureNoEcsError(err)

	var attachmentID *string
	for _, container := range resp.Tasks[0].Containers {
		if *container.Name == cliContainerName {
			attachmentID = container.NetworkInterfaces[0].AttachmentId
		}
	}

	if attachmentID == nil {
		fmt.Println("Unable to determine IP address.")
		panic("Error; exiting.")
	}

	var theID *string

	for _, attachment := range resp.Tasks[0].Attachments {
		if *attachment.Id == *attachmentID {
			var keyToFind string
			if publicIP {
				keyToFind = "networkInterfaceId"
			} else {
				keyToFind = "privateIPv4Address"
			}
			for _, kv := range attachment.Details {
				if *kv.Name == keyToFind {
					theID = kv.Value
				}
			}
		}
	}

	if !publicIP {
		return theID
	}

	ec2Svc := ec2.New(sess)
	netIntResp, err := ec2Svc.DescribeNetworkInterfaces(&ec2.DescribeNetworkInterfacesInput{
		NetworkInterfaceIds: []*string{
			theID,
		},
	})
	ensureNoEcsError(err)

	ipAddress := netIntResp.NetworkInterfaces[0].Association.PublicIp

	return ipAddress
}

func ecsTaskError(ecsSvc *ecs.ECS, describeTaskInput *ecs.DescribeTasksInput) {
	resp, err := ecsSvc.DescribeTasks(describeTaskInput)
	ensureNoEcsError(err)
	task := resp.Tasks[0]
	out := []string{}
	out = append(out, fmt.Sprintf("Task in status %s", *task.LastStatus))
	if *task.LastStatus == "STOPPED" {
		out = append(out, fmt.Sprintf("Stopped reason: %s", *task.StoppedReason))
	}
	for _, container := range task.Containers {
		out = append(out, fmt.Sprintf("[%s] Status: %s", *container.Name, *container.LastStatus))
		out = append(out, fmt.Sprintf("[%s] Status reason: %s", *container.Name, *container.Reason))
	}
	panic(fmt.Errorf(strings.Join(out, "\n")))
}

func ensureNoEcsError(err error) {
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case ecs.ErrCodeServerException:
				fmt.Println(ecs.ErrCodeServerException, aerr.Error())
			case ecs.ErrCodeClientException:
				fmt.Println(ecs.ErrCodeClientException, aerr.Error())
			case ecs.ErrCodeInvalidParameterException:
				fmt.Println(ecs.ErrCodeInvalidParameterException, aerr.Error())
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
		panic("Error; exiting.")
	}
}
