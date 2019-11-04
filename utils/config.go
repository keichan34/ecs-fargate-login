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
	"flag"
	"strings"
)

// ECSTaskConfiguration contains configuration by the command line opts
type ECSTaskConfiguration struct {
	TaskDefinitionName string
	ClusterName        string
	CliContainerName   string

	AssignPublicIP bool

	SecurityGroups []string
	Subnets        []string
}

// ParseOpts will parse command line arguments in to a ECSTaskConfiguration struct.
func ParseOpts() (*ECSTaskConfiguration, error) {
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
		return nil, errors.New("required fields not included")
	}

	return &ECSTaskConfiguration{
		TaskDefinitionName: taskDefinitionName,
		ClusterName:        clusterName,
		CliContainerName:   cliContainerName,
		AssignPublicIP:     assignPublicIP,
		SecurityGroups:     strings.Split(securityGroupsStr, ","),
		Subnets:            strings.Split(subnetsStr, ","),
	}, nil
}
