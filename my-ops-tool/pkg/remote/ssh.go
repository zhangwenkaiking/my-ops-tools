package remote

import (
	"fmt"
	"golang.org/x/crypto/ssh"
	"time"
)

type Task struct {
	IP      string
	Command string
}

type Executor struct {
	User     string
	Password string
}

func (e *Executor) Run(task Task) (string, error) {
	config := &ssh.ClientConfig{
		User: e.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(e.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:22", task.IP)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return "", fmt.Errorf("拨号失败 [%s]: %w", task.IP, err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("会话失败 [%s]: %w", task.IP, err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(task.Command)
	if err != nil {
		return "", fmt.Errorf("执行失败 [%s]: %w, 输出: %s", task.IP, err, string(output))
	}

	return string(output), nil
}
