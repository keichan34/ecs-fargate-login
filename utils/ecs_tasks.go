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

package utils

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
)

func createSession() *session.Session {
	return session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
}

// StartTaskInput contains the input for StartTask
type StartTaskInput struct {
	Configuration *ECSTaskConfiguration
	KeyPair       *SSHKeyPair
}

// StartTaskOutput contains the output for StartTask
type StartTaskOutput struct {
	TaskArn       *string
	TaskIPAddress *string
}

// StartTask starts a task and returns information about it.
func StartTask(input *StartTaskInput) (*StartTaskOutput, error) {
	sess := createSession()

	taskArn, err := runTask(sess, input)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Started task with ARN: %s\n", *taskArn)

	taskIP, err := getTaskIPAddress(sess, input, taskArn)
	if err != nil {
		return nil, err
	}

	if taskIP == nil {
		return nil, errors.New("couldn't determine task IP address")
	}

	return &StartTaskOutput{
		TaskArn:       taskArn,
		TaskIPAddress: taskIP,
	}, nil
}

func runTask(sess *session.Session, input *StartTaskInput) (*string, error) {
	ecsSvc := ecs.New(sess)

	var assignPublicIPStr string
	if input.Configuration.AssignPublicIP == true {
		assignPublicIPStr = "ENABLED"
	} else {
		assignPublicIPStr = "DISABLED"
	}

	resp, err := ecsSvc.RunTask(&ecs.RunTaskInput{
		TaskDefinition: aws.String(input.Configuration.TaskDefinitionName),
		Cluster:        aws.String(input.Configuration.ClusterName),
		StartedBy:      aws.String("ecs-fargate-login"),
		Overrides: &ecs.TaskOverride{
			ContainerOverrides: []*ecs.ContainerOverride{
				&ecs.ContainerOverride{
					Name: aws.String(input.Configuration.CliContainerName),
					Environment: []*ecs.KeyValuePair{
						&ecs.KeyValuePair{
							Name:  aws.String("_AUTHORIZED_PUBLIC_KEY"),
							Value: aws.String(input.KeyPair.PublicKeyAuthorizedKey),
						},
					},
				},
			},
		},
		LaunchType: aws.String("FARGATE"),
		NetworkConfiguration: &ecs.NetworkConfiguration{
			AwsvpcConfiguration: &ecs.AwsVpcConfiguration{
				AssignPublicIp: aws.String(assignPublicIPStr),
				SecurityGroups: aws.StringSlice(input.Configuration.SecurityGroups),
				Subnets:        aws.StringSlice(input.Configuration.Subnets),
			},
		},
	})
	if err != nil {
		return nil, err
	}

	task := resp.Tasks[0]
	return task.TaskArn, nil
}

func getTaskIPAddress(sess *session.Session, input *StartTaskInput, taskArn *string) (*string, error) {
	ecsSvc := ecs.New(sess)

	describeTaskInput := &ecs.DescribeTasksInput{
		Cluster: aws.String(input.Configuration.ClusterName),
		Tasks: []*string{
			taskArn,
		},
	}
	err := ecsSvc.WaitUntilTasksRunningWithContext(
		aws.BackgroundContext(),
		describeTaskInput,
		request.WithWaiterDelay(request.ConstantWaiterDelay(5*time.Second)),
		request.WithWaiterMaxAttempts(60), // 5 minutes = 5 seconds * 60
	)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == request.WaiterResourceNotReadyErrorCode {
			ecsTaskError(ecsSvc, describeTaskInput)
		}
		return nil, err
	}

	resp, err := ecsSvc.DescribeTasks(describeTaskInput)
	if err != nil {
		return nil, err
	}

	var attachmentID *string
	for _, container := range resp.Tasks[0].Containers {
		if *container.Name == input.Configuration.CliContainerName {
			attachmentID = container.NetworkInterfaces[0].AttachmentId
		}
	}

	if attachmentID == nil {
		return nil, errors.New("unable to determine IP address")
	}

	var theID *string
	var keyToFind string
	if input.Configuration.AssignPublicIP {
		keyToFind = "networkInterfaceId"
	} else {
		keyToFind = "privateIPv4Address"
	}

	for _, attachment := range resp.Tasks[0].Attachments {
		if *attachment.Id == *attachmentID {
			for _, kv := range attachment.Details {
				if *kv.Name == keyToFind {
					theID = kv.Value
				}
			}
		}
	}

	// If the public IP is requested, we have to continue using the EC2 API to determine
	// the public IP address using the networkInterfaceId.
	if !input.Configuration.AssignPublicIP {
		return theID, nil
	}

	ec2Svc := ec2.New(sess)
	netIntResp, err := ec2Svc.DescribeNetworkInterfaces(&ec2.DescribeNetworkInterfacesInput{
		NetworkInterfaceIds: []*string{
			theID,
		},
	})
	if err != nil {
		return nil, err
	}

	ipAddress := netIntResp.NetworkInterfaces[0].Association.PublicIp

	return ipAddress, nil
}

func ecsTaskError(ecsSvc *ecs.ECS, describeTaskInput *ecs.DescribeTasksInput) {
	resp, err := ecsSvc.DescribeTasks(describeTaskInput)
	FormatErrorAndExit(err)
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
}

// CleanupTask will poll until the task is stopped. If it hasn't stopped within one minute,
// it will forcibly stop the task by using the StopTask API.
func CleanupTask(config *ECSTaskConfiguration, taskArn *string) {
	sess := createSession()
	ecsSvc := ecs.New(sess)

	describeTaskInput := &ecs.DescribeTasksInput{
		Cluster: aws.String(config.ClusterName),
		Tasks: []*string{
			taskArn,
		},
	}
	err := ecsSvc.WaitUntilTasksStoppedWithContext(
		aws.BackgroundContext(),
		describeTaskInput,
		request.WithWaiterDelay(request.ConstantWaiterDelay(5*time.Second)),
		request.WithWaiterMaxAttempts(12), // 5 seconds * 12 = 1 minute
	)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == request.WaiterResourceNotReadyErrorCode {
			fmt.Println("Task hasn't stopped within 1 minute; forcibly stopping task...")
			forciblyStopTask(sess, config.ClusterName, taskArn)
		}
	}
	FormatErrorAndExit(err)
}

func forciblyStopTask(sess *session.Session, clusterName string, taskArn *string) {
	ecsSvc := ecs.New(sess)

	stopTaskInput := &ecs.StopTaskInput{
		Cluster: aws.String(clusterName),
		Task:    taskArn,
	}
	_, err := ecsSvc.StopTask(stopTaskInput)
	FormatErrorAndExit(err)

	fmt.Println("Task stopped.")
}

// FormatErrorAndExit will print the error (if error) to stdout and exit 1.
func FormatErrorAndExit(err error) {
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
			fmt.Fprintln(os.Stderr, err.Error())
		}
		os.Exit(1)
	}
}
