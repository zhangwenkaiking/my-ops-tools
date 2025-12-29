package config

import (
	"fmt"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	MasterIP            string   `mapstructure:"master_ip"`
	NodeIPs             []string `mapstructure:"node_ips"`
	SSHUser             string   `mapstructure:"ssh_user"`
	SSHPassword         string
	AnsiblePlaybookPath string `mapstructure:"ansible_playbook_path"`
	K8sVersion          string `mapstructure:"k8s_version"`
}

func LoadConfig() (*Config, error) {
	// 1. 使用绝对路径加载 .env 增强兼容性
	execPath, _ := os.Executable()
	baseDir := filepath.Dir(execPath)
	_ = godotenv.Load(filepath.Join(baseDir, ".env")) // 尝试在可执行文件目录下找
	_ = godotenv.Load()                               // 同时也尝试在当前运行目录下找

	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	conf := &Config{}
	if err := v.Unmarshal(conf); err != nil {
		return nil, err
	}

	// 2. 依次读取密码：环境变量优先
	pass := os.Getenv("SSH_PASSWORD")
	if pass == "" {
		// 如果环境变量没有，尝试从 config.yaml 中的 ssh_password 字段读取
		pass = v.GetString("ssh_password")
	}

	// 3. 核心校验：如果最终还是没拿到密码，直接报错
	if pass == "" {
		return nil, fmt.Errorf("❌ 严重错误: 未能获取到 SSH 密码！\n请检查: \n1. .env 文件中是否有 SSH_PASSWORD=xxx \n2. 或 config.yaml 中是否有 ssh_password: xxx")
	}

	conf.SSHPassword = pass
	// 调试信息：确认读取到了密码（只显示前2位脱敏）
	fmt.Printf("🔐 认证信息加载成功，密码长度: %d 位\n", len(pass))

	return conf, nil
}

// SaveToInventory 生成并保存 hosts.ini
func (c *Config) SaveToInventory() (string, error) {
	var sb strings.Builder
	filename := "hosts.ini"

	// 防御性编程：再次检查密码是否为空
	if c.SSHPassword == "" {
		return "", fmt.Errorf("无法生成 Inventory: 内存中 SSH 密码为空")
	}

	sb.WriteString("[master]\n")
	sb.WriteString(fmt.Sprintf("%s ansible_user=%s ansible_ssh_pass=%s\n\n", c.MasterIP, c.SSHUser, c.SSHPassword))

	sb.WriteString("[nodes]\n")
	for _, ip := range c.NodeIPs {
		sb.WriteString(fmt.Sprintf("%s ansible_user=%s ansible_ssh_pass=%s\n", ip, c.SSHUser, c.SSHPassword))
	}

	err := os.WriteFile(filename, []byte(sb.String()), 0644)
	return filename, err
}

func (c *Config) RemoveNode(ip string) {
	var newNodes []string
	for _, n := range c.NodeIPs {
		if n != ip {
			newNodes = append(newNodes, n)
		}
	}
	c.NodeIPs = newNodes
}
