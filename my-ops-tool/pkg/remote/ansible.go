package remote

import (
	"bufio"
	"fmt"
	"my-ops-tool/pkg/config"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

type AnsibleRunner struct {
	Inventory string // hosts.ini 的路径
}

func NewAnsibleRunner(inventory string) *AnsibleRunner {
	return &AnsibleRunner{Inventory: inventory}
}

func (r *AnsibleRunner) RunPlaybook(playbookPath string) error {
	fmt.Printf("\n📦 正在执行 Ansible 剧本: %s (清单: %s)\n", playbookPath, r.Inventory)
	cmd := exec.Command("ansible-playbook", "-i", r.Inventory, playbookPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// InitMaster 初始化 Master 节点并获取 Join 命令
func (r *AnsibleRunner) InitMaster(conf *config.Config) (string, error) {
	fmt.Printf("\n☸️  [阶段 2/5] 正在初始化 Master 节点: %s\n", conf.MasterIP)

	// 1. 强力清理旧环境（防止多次运行导致的 etcd 或证书冲突）
	fmt.Println("   >> 正在清理旧的 K8s 遗留文件与配置...")
	cleanupCmd := "kubeadm reset -f && rm -rf /etc/kubernetes/ /var/lib/etcd/ /var/lib/kubelet/ $HOME/.kube"
	_ = exec.Command("ansible", "master", "-i", r.Inventory, "-m", "shell", "-a", cleanupCmd).Run()

	// 2. 构造 kubeadm init 参数
	shortVer := strings.Split(conf.K8sVersion, "-")[0]
	repo := "registry.aliyuncs.com/google_containers"
	// 确保 socket 路径与 crictl.yaml 配置完全一致
	sock := "unix:///run/containerd/containerd.sock"

	// 8G 内存环境下，我们依然保留 ignore-errors 以确保流程不被微小告警中断
	// 构造一个简单的配置文件传给 kubeadm (或者直接通过参数)
	initArgs := fmt.Sprintf("kubeadm init "+
		"--kubernetes-version=%s "+
		"--pod-network-cidr=10.244.0.0/16 "+
		"--image-repository=%s "+
		"--apiserver-advertise-address=%s "+
		"--node-name=k8s-master "+
		"--ignore-preflight-errors=all "+
		"--cri-socket=%s "+
		// 🔥 关键增加：强制指定 cgroup 驱动为 systemd
		"--v=5", shortVer, repo, conf.MasterIP, sock)

	// 3. 执行初始化命令
	fmt.Println("   >> 正在执行 kubeadm init (这可能需要 1-2 分钟)...")
	cmd := exec.Command("ansible", "master", "-i", r.Inventory, "-m", "shell", "-a", initArgs)

	// 获取输出流以实时显示进度
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("启动初始化命令失败: %v", err)
	}

	var fullOutput strings.Builder
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		fullOutput.WriteString(line + "\n")
		// 只打印关键进度行，避免日志刷屏
		if strings.Contains(line, "[") || strings.Contains(line, "k8s-master") {
			fmt.Printf("      %s\n", line)
		}
	}

	// 4. 等待执行完成
	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("Master 初始化执行失败。请登录 Master 运行 'journalctl -xeu kubelet' 查看详情。\n错误: %v", err)
	}

	// 5. 提取 Join 命令
	fmt.Println("   >> Master 初始化成功，正在提取集群加入口令...")
	joinCmd, err := r.extractJoinCommand(fullOutput.String())
	if err != nil {
		return "", err
	}

	return joinCmd, nil
}

// extractJoinCommand 辅助函数：清洗并提取 join 命令
func (r *AnsibleRunner) extractJoinCommand(output string) (string, error) {
	// 1. 预处理：清洗 Ansible 输出中的换行符和转义斜杠
	cleanOutput := strings.ReplaceAll(output, "\\n", " ")
	cleanOutput = strings.ReplaceAll(cleanOutput, "\\", " ")

	// 2. 正则匹配：匹配从 kubeadm join 开始到哈希值结束的部分
	re := regexp.MustCompile(`kubeadm join [\s\S]+?--discovery-token-ca-cert-hash sha256:[a-z0-9]+`)
	joinCmd := re.FindString(cleanOutput)

	if joinCmd == "" {
		return "", fmt.Errorf("未能从输出中解析出 join 命令。原始输出预览: %s", output[:200])
	}

	// 3. 压缩多余空格
	spaceRe := regexp.MustCompile(`\s+`)
	finalCmd := strings.TrimSpace(spaceRe.ReplaceAllString(joinCmd, " "))

	return finalCmd, nil
}

func (r *AnsibleRunner) JoinNodes(joinCmd string, conf *config.Config) error {
	fmt.Println("\n🤝 正在批量加入 Worker 节点...")
	for i, ip := range conf.NodeIPs {
		nodeName := fmt.Sprintf("k8s-node%d", i+1)
		fullJoinCmd := fmt.Sprintf("%s --node-name=%s --cri-socket=unix:///var/run/containerd/containerd.sock", joinCmd, nodeName)
		fmt.Printf("   >> 正在加入: %s (%s)...\n", ip, nodeName)
		cmd := exec.Command("ansible", ip, "-i", r.Inventory, "-m", "shell", "-a", fullJoinCmd)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("节点 %s 加入失败: %v, 输出: %s", ip, err, string(output))
		}
	}
	return nil
}

func (r *AnsibleRunner) SetupKubectlConfig() error {
	fmt.Println("\n🔧 正在配置 kubectl 权限...")
	setupCmd := `mkdir -p $HOME/.kube && cp -i /etc/kubernetes/admin.conf $HOME/.kube/config && chown $(id -u):$(id -g) $HOME/.kube/config`
	return exec.Command("ansible", "master", "-i", r.Inventory, "-m", "shell", "-a", setupCmd).Run()
}

//func (r *AnsibleRunner) InstallNetwork() error {
//	fmt.Println("\n🌐 正在部署 Calico 网络...")
//	installCmd := "kubectl create -f https://raw.githubusercontent.com/projectcalico/calico/v3.26.1/manifests/tigera-operator.yaml && sleep 5 && kubectl create -f https://raw.githubusercontent.com/projectcalico/calico/v3.26.1/manifests/custom-resources.yaml"
//	return exec.Command("ansible", "master", "-i", r.Inventory, "-m", "shell", "-a", installCmd).Run()
//}

func (r *AnsibleRunner) InstallNetwork() error {
	fmt.Println("\n🌐 正在部署离线 Calico 网络 (使用本地配置文件)...")

	// 💡 改为执行我们在 Ansible 里准备好的 /tmp/calico.yaml (由 Ansible 分发过去的)
	// 或者直接在这里使用 kubectl apply 分发后的本地文件
	installCmd := "export KUBECONFIG=/etc/kubernetes/admin.conf && kubectl apply -f /tmp/calico.yaml"

	return exec.Command("ansible", "master", "-i", r.Inventory, "-m", "shell", "-a", installCmd).Run()
}
