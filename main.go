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
	"fmt"
	"os"
	"os/exec"

	"github.com/keichan34/ecs-fargate-login/utils"
)

func main() {
	config, err := utils.ParseOpts()
	if err != nil {
		os.Exit(1)
	}

	keyPair, err := utils.GenerateSSHKeyPair()
	utils.FormatErrorAndExit(err)

	taskInfo, err := utils.StartTask(&utils.StartTaskInput{
		Configuration: config,
		KeyPair:       keyPair,
	})
	utils.FormatErrorAndExit(err)
	defer utils.CleanupTask(config, taskInfo.TaskArn)

	fmt.Printf("Task %s is running at %s!\n", *taskInfo.TaskArn, *taskInfo.TaskIPAddress)

	tmpfile, err := utils.WritePrivateKeyToTempfile(keyPair)
	utils.FormatErrorAndExit(err)
	defer os.Remove(tmpfile.Name())

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
		fmt.Sprintf("root@%s", *taskInfo.TaskIPAddress), // <- TODO: support different user
	)
	// redirect the output to terminal
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.Run()

	fmt.Println("Disconnected from SSH. Waiting for task to stop...")
}
