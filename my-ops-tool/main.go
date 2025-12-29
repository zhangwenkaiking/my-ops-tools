package main

import (
	"fmt"
	"log"
	"my-ops-tool/pkg/config"
	"my-ops-tool/pkg/remote"
	"os"
)

func main() {
	fmt.Println("==================================================")
	fmt.Println("🚀 K8s 离线全自动部署工具启动 (v1.28.2)")
	fmt.Println("==================================================")

	// 1. 加载配置
	conf, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("❌ 配置加载失败: %v", err)
	}

	// 2. 🔥 关键步骤：生成 Ansible Inventory (hosts.ini)
	fmt.Println("📝 正在生成主机清单 (Inventory)...")
	inventoryFile, err := conf.SaveToInventory()
	if err != nil {
		log.Fatalf("❌ 生成 hosts.ini 失败: %v", err)
	}

	// 3. 初始化 Runner
	runner := remote.NewAnsibleRunner(inventoryFile)

	// 4. 阶段 1：环境初始化与离线镜像导入
	playbook := "k8s_init.yml"
	if _, err := os.Stat(playbook); os.IsNotExist(err) {
		log.Fatalf("❌ 找不到剧本文件: %s", playbook)
	}

	fmt.Println("\n[1/5] 正在执行系统初始化与镜像分发 (请耐心等待)...")
	if err := runner.RunPlaybook(playbook); err != nil {
		log.Fatalf("❌ 基础环境初始化失败: %v", err)
	}

	// 5. 阶段 2：Master 初始化
	fmt.Println("\n[2/5] 正在启动控制平面 (Master)...")
	joinCmd, err := runner.InitMaster(conf)
	if err != nil {
		log.Fatalf("❌ Master 初始化失败: %v", err)
	}
	fmt.Println("✅ Master 节点就绪。")

	// 6. 阶段 3：权限配置
	err = runner.SetupKubectlConfig()
	if err != nil {
		log.Printf("⚠️ 警告：配置 kubectl 权限失败: %v", err)
	}

	// 7. 阶段 4：Worker 节点加入
	fmt.Println("\n[4/5] 正在将 Worker 节点批量加入集群...")
	if err := runner.JoinNodes(joinCmd, conf); err != nil {
		log.Fatalf("❌ Worker 加入失败: %v", err)
	}

	// 8. 阶段 5：安装 Calico 网络
	fmt.Println("\n[5/5] 正在部署 Calico 网络策略...")
	if err := runner.InstallNetwork(); err != nil {
		log.Fatalf("❌ 网络安装失败: %v", err)
	}

	fmt.Println("\n==================================================")
	fmt.Println("🎉 集群部署成功！")
	fmt.Println("👉 可登录 Master 节点运行 'kubectl get nodes' 查看。")
	fmt.Println("==================================================")
}
