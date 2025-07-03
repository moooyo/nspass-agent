# NSPass Agent Makefile

# 版本信息
VERSION ?= $(shell git describe --tags --always --dirty)
COMMIT ?= $(shell git rev-parse --short HEAD)
BUILD_TIME ?= $(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Go构建参数
GOOS ?= linux
GOARCH ?= amd64
CGO_ENABLED ?= 0

# 构建标志
LDFLAGS = -w -s \
	-X 'main.Version=$(VERSION)' \
	-X 'main.Commit=$(COMMIT)' \
	-X 'main.BuildTime=$(BUILD_TIME)'

# 二进制文件输出路径
BINARY_NAME = nspass-agent
OUTPUT_DIR = dist
BINARY_PATH = $(OUTPUT_DIR)/$(BINARY_NAME)

# Proto相关路径
PROTO_DIR = proto
GENERATED_DIR = generated
PROTO_FILES = $(shell find $(PROTO_DIR) -name "*.proto")

# 安装路径
INSTALL_PATH = /usr/local/bin
CONFIG_PATH = /etc/nspass
SYSTEMD_PATH = /etc/systemd/system

.PHONY: all build clean test install uninstall deps lint format help proto-deps proto-gen proto-clean

# 默认目标
all: proto-clean proto-gen build

# 构建二进制文件
build: proto-gen
	@echo "构建 $(BINARY_NAME)..."
	@mkdir -p $(OUTPUT_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -ldflags "$(LDFLAGS)" -o $(BINARY_PATH) ./cmd/$(BINARY_NAME)
	@echo "构建完成: $(BINARY_PATH)"

# 清理构建文件
clean: proto-clean
	@echo "清理构建文件..."
	@rm -rf $(OUTPUT_DIR)
	@go clean

# 运行测试
test:
	@echo "运行测试..."
	@go test -v ./...

# 安装依赖
deps:
	@echo "安装依赖..."
	@go mod download
	@go mod verify

# 代码检查
lint:
	@echo "运行代码检查..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "请安装 golangci-lint"; \
		exit 1; \
	fi

# 格式化代码
format:
	@echo "格式化代码..."
	@go fmt ./...
	@go vet ./...

# 安装到系统
install: build
	@echo "安装 $(BINARY_NAME) 到系统..."
	@sudo cp $(BINARY_PATH) $(INSTALL_PATH)/
	@sudo chmod +x $(INSTALL_PATH)/$(BINARY_NAME)
	
	@echo "创建配置目录..."
	@sudo mkdir -p $(CONFIG_PATH)
	@sudo mkdir -p $(CONFIG_PATH)/proxy
	@sudo cp configs/config.yaml $(CONFIG_PATH)/config.yaml.example
	
	@echo "安装systemd服务..."
	@sudo cp systemd/nspass-agent.service $(SYSTEMD_PATH)/
	@sudo systemctl daemon-reload
	
	@echo "安装完成！"
	@echo "请配置 $(CONFIG_PATH)/config.yaml 后启动服务："
	@echo "  sudo systemctl enable nspass-agent"
	@echo "  sudo systemctl start nspass-agent"

# 从系统卸载
uninstall:
	@echo "卸载 $(BINARY_NAME)..."
	@sudo systemctl stop nspass-agent 2>/dev/null || true
	@sudo systemctl disable nspass-agent 2>/dev/null || true
	@sudo rm -f $(SYSTEMD_PATH)/nspass-agent.service
	@sudo rm -f $(INSTALL_PATH)/$(BINARY_NAME)
	@sudo systemctl daemon-reload
	@echo "卸载完成！"
	@echo "注意：配置文件保留在 $(CONFIG_PATH)/ 中"

# 构建多架构版本
build-all:
	@echo "构建多架构版本..."
	@mkdir -p $(OUTPUT_DIR)
	
	@echo "构建 linux/amd64..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -ldflags "$(LDFLAGS)" -o $(OUTPUT_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/$(BINARY_NAME)
	
	@echo "构建 linux/arm64..."
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
		go build -ldflags "$(LDFLAGS)" -o $(OUTPUT_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/$(BINARY_NAME)
	
	@echo "构建 linux/386..."
	CGO_ENABLED=0 GOOS=linux GOARCH=386 \
		go build -ldflags "$(LDFLAGS)" -o $(OUTPUT_DIR)/$(BINARY_NAME)-linux-386 ./cmd/$(BINARY_NAME)

# 创建发布包
package: build-all
	@echo "创建发布包..."
	@cd $(OUTPUT_DIR) && \
		for binary in $(BINARY_NAME)-*; do \
			tar -czf $$binary.tar.gz $$binary; \
		done
	@echo "发布包创建完成！"

# 开发模式运行
dev:
	@echo "开发模式运行..."
	@go run ./cmd/$(BINARY_NAME) --config configs/config.yaml --log-level debug

# 显示帮助信息
help:
	@echo "NSPass Agent 构建系统"
	@echo ""
	@echo "可用目标："
	@echo "  build      - 构建二进制文件"
	@echo "  clean      - 清理构建文件"
	@echo "  test       - 运行测试"
	@echo "  deps       - 安装依赖"
	@echo "  lint       - 代码检查"
	@echo "  format     - 格式化代码"
	@echo "  install    - 安装到系统"
	@echo "  uninstall  - 从系统卸载"
	@echo "  build-all  - 构建多架构版本"
	@echo "  package    - 创建发布包"
	@echo "  dev        - 开发模式运行"
	@echo "  proto-deps - 安装proto工具依赖"
	@echo "  proto-gen  - 生成proto代码"
	@echo "  proto-clean- 清理生成的proto代码"
	@echo "  help       - 显示此帮助信息"
	@echo ""
	@echo "环境变量："
	@echo "  VERSION    - 版本号 (默认: git tag)"
	@echo "  GOOS       - 目标操作系统 (默认: linux)"
	@echo "  GOARCH     - 目标架构 (默认: amd64)"

# ===== Proto相关目标 =====

# 安装proto工具依赖
proto-deps:
	@echo "安装proto工具依赖..."
	@if ! command -v protoc >/dev/null 2>&1; then \
		echo "错误: protoc未安装，请先安装Protocol Buffers"; \
		echo "macOS: brew install protobuf"; \
		echo "Ubuntu: apt-get install protobuf-compiler"; \
		exit 1; \
	fi
	@if ! command -v protoc-gen-go >/dev/null 2>&1; then \
		echo "安装protoc-gen-go..."; \
		go install google.golang.org/protobuf/cmd/protoc-gen-go@latest; \
	fi
	@if ! command -v protoc-gen-go-grpc >/dev/null 2>&1; then \
		echo "安装protoc-gen-go-grpc..."; \
		go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest; \
	fi
	@echo "proto工具依赖安装完成！"

# 清理生成的proto代码
proto-clean:
	@echo "清理生成的proto代码..."
	@rm -rf $(GENERATED_DIR)

# 生成proto代码
proto-gen: proto-deps
	@echo "生成proto代码..."
	@mkdir -p $(GENERATED_DIR)
	@export PATH="$$PATH:$(shell go env GOPATH)/bin"; \
	for proto in $(PROTO_FILES); do \
		echo "处理: $$proto"; \
		protoc \
			--proto_path=$(PROTO_DIR) \
			--proto_path=$(PROTO_DIR)/google/protobuf \
			--go_out=$(GENERATED_DIR) \
			--go_opt=paths=source_relative \
			--go-grpc_out=$(GENERATED_DIR) \
			--go-grpc_opt=paths=source_relative \
			$$proto; \
	done
	@echo "proto代码生成完成！" 