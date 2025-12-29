# cat pullImage.sh
#!/bin/bash

# --- 版本定义 ---
K8S_VERSION="v1.28.2"
AL_REPO="registry.aliyuncs.com/google_containers"
CALICO_VERSION="v3.26.1"

# --- 镜像清单 ---
# 1. K8s 核心组件 (使用阿里云前缀)
k8s_images=(
    "kube-apiserver:$K8S_VERSION"
    "kube-controller-manager:$K8S_VERSION"
    "kube-scheduler:$K8S_VERSION"
    "kube-proxy:$K8S_VERSION"
    "pause:3.9"
    "etcd:3.5.9-0"
    "coredns:v1.10.1"
)

# 2. Calico 核心组件 (官方镜像)
calico_images=(
    "calico/cni:$CALICO_VERSION"
    "calico/node:$CALICO_VERSION"
    "calico/kube-controllers:$CALICO_VERSION"
    "calico/typha:$CALICO_VERSION"
)

echo "🚀 开始拉取 K8s $K8S_VERSION 镜像..."
for img in "${k8s_images[@]}"; do
    docker pull "$AL_REPO/$img"
done

echo "🌐 开始拉取 Calico $CALICO_VERSION 镜像..."
for img in "${calico_images[@]}"; do
    docker pull "docker.io/$img"
done

echo "📦 正在打包全量镜像 (K8s + Calico)..."
# 匹配所有拉取的镜像并保存
docker save $(docker images | grep -E "google_containers|calico" | awk '{print $1":"$2}') -o k8s-calico-full-bundle.tar

echo "✅ 打包完成！文件: k8s-calico-full-bundle.tar"
echo "👉 请将此文件放入 Go 项目的 resources 目录。"
