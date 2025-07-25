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
PROTO_FILES = $(shell find $(PROTO_DIR) -name "*.proto" 2>/dev/null || echo "")

# 安装路径
INSTALL_PATH = /usr/local/bin
CONFIG_PATH = /etc/nspass
SYSTEMD_PATH = /etc/systemd/system

# 设置PATH环境变量包含Go bin目录
export PATH := $(PATH):$(shell go env GOPATH)/bin

# Protobuf工具的版本要求
PROTOC_GEN_GO_VERSION := latest
PROTOC_GEN_GO_GRPC_VERSION := latest
PROTOC_GEN_GRPC_GATEWAY_VERSION := latest
PROTOC_GEN_OPENAPIV2_VERSION := latest

# Proto文件监听标记文件
PROTO_TIMESTAMP_FILE := .proto_timestamp

.PHONY: all build clean deep-clean test install uninstall deps lint format help gen-proto gen-proto-force proto-clean build-all release release-github run check-proto-tools install-proto-tools check-proto-changed gen-proto-internal check-proto-env

# 默认目标
all: proto-clean gen-proto build

# 构建二进制文件
build: gen-proto
	@echo "构建 $(BINARY_NAME)..."
	@mkdir -p $(OUTPUT_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -ldflags "$(LDFLAGS)" -o $(BINARY_PATH) ./cmd/$(BINARY_NAME)
	@echo "构建完成: $(BINARY_PATH)"

# 清理构建文件
clean:
	@echo "清理构建文件..."
	@rm -rf $(OUTPUT_DIR)
	@rm -f $(BINARY_NAME)
	@go clean

# 深度清理（包括生成的代码和缓存）
deep-clean: clean proto-clean
	@echo "深度清理项目..."
	@rm -rf build/
	@rm -rf dist/
	@rm -rf release/
	@rm -rf $(GENERATED_DIR)
	@rm -f $(PROTO_TIMESTAMP_FILE)
	@go clean -cache
	@go clean -modcache
	@echo "深度清理完成"

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

# 检查和安装protobuf工具
check-proto-tools:
	@echo "🔍 检查 protobuf 工具..."
	@if ! command -v protoc >/dev/null 2>&1; then \
		echo "❌ protoc 未安装，请先安装 Protocol Buffers"; \
		echo "Ubuntu/Debian: sudo apt-get install protobuf-compiler"; \
		echo "macOS: brew install protobuf"; \
		echo "或访问: https://grpc.io/docs/protoc-installation/"; \
		exit 1; \
	else \
		echo "✅ protoc 已安装: $$(protoc --version)"; \
	fi

# 安装protobuf Go插件
install-proto-tools:
	@echo "📦 安装/更新 protobuf Go 插件..."
	@GOBIN=$(shell go env GOPATH)/bin go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	@GOBIN=$(shell go env GOPATH)/bin go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)
	@GOBIN=$(shell go env GOPATH)/bin go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@$(PROTOC_GEN_GRPC_GATEWAY_VERSION)
	@GOBIN=$(shell go env GOPATH)/bin go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@$(PROTOC_GEN_OPENAPIV2_VERSION)
	@echo "✅ protobuf Go 插件安装完成"
	@echo "🔍 验证插件安装..."
	@for tool in protoc-gen-go protoc-gen-go-grpc protoc-gen-grpc-gateway protoc-gen-openapiv2; do \
		if command -v $$tool >/dev/null 2>&1; then \
			echo "  ✅ $$tool: $$($$tool --version 2>/dev/null || echo 'installed')"; \
		else \
			echo "  ❌ $$tool: 未找到"; \
		fi; \
	done

# 检查proto文件是否需要重新生成
check-proto-changed:
	@if [ ! -f $(PROTO_TIMESTAMP_FILE) ]; then \
		echo "📝 首次运行，需要生成 proto 文件"; \
		exit 1; \
	fi
	@if [ ! -d $(GENERATED_DIR) ] || [ ! -f $(GENERATED_DIR)/go.mod ]; then \
		echo "📝 检测到生成目录不完整，需要重新生成"; \
		exit 1; \
	fi
	@if [ -n "$(PROTO_FILES)" ]; then \
		for proto_file in $(PROTO_FILES); do \
			if [ "$$proto_file" -nt $(PROTO_TIMESTAMP_FILE) ]; then \
				echo "📝 检测到 proto 文件变更: $$proto_file"; \
				exit 1; \
			fi; \
		done; \
	fi
	@echo "✅ Proto 文件未变更，跳过生成步骤"

# 生成proto文件的内部实现
gen-proto-internal: check-proto-tools install-proto-tools
	@echo "🚀 生成 protobuf 代码..."
	@echo "PROTO_DIR: $(PROTO_DIR)"
	@echo "GENERATED_DIR: $(GENERATED_DIR)"
	@mkdir -p $(GENERATED_DIR)
	@echo "找到的proto文件数量: $(shell find $(PROTO_DIR) -name '*.proto' | wc -l)"
	@if [ "$(shell find $(PROTO_DIR) -name '*.proto' | wc -l)" = "0" ]; then \
		echo "❌ 没有找到任何proto文件! 请确保proto子模块已正确初始化:"; \
		echo "   git submodule update --init --recursive"; \
		exit 1; \
	fi
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
			$$proto || { echo "❌ 生成 $$proto 失败，退出码: $$?"; exit 1; }; \
		echo "✅ 完成处理: $$proto"; \
	done
	@echo "检查生成的文件..."
	@ls -la $(GENERATED_DIR)/ || echo "generated目录为空"
	@if [ -d $(GENERATED_DIR)/model ]; then \
		echo "model目录内容:"; \
		ls -la $(GENERATED_DIR)/model/; \
	else \
		echo "⚠️  model目录不存在"; \
	fi
	@echo "确保生成的代码不创建独立的go.mod..."
	@rm -f $(GENERATED_DIR)/go.mod
	@echo "创建适用于replace的go.mod文件..."
	@echo "module github.com/moooyo/nspass-proto/generated" > $(GENERATED_DIR)/go.mod
	@echo "" >> $(GENERATED_DIR)/go.mod
	@echo "go 1.24" >> $(GENERATED_DIR)/go.mod
	@echo "" >> $(GENERATED_DIR)/go.mod
	@echo "require (" >> $(GENERATED_DIR)/go.mod
	@echo "	google.golang.org/protobuf v1.36.6" >> $(GENERATED_DIR)/go.mod
	@echo "	google.golang.org/genproto/googleapis/api v0.0.0-20250715232539-7130f93afb79" >> $(GENERATED_DIR)/go.mod
	@echo "	google.golang.org/grpc v1.73.0" >> $(GENERATED_DIR)/go.mod
	@echo ")" >> $(GENERATED_DIR)/go.mod
	@echo "" >> $(GENERATED_DIR)/go.mod
	@echo "require (" >> $(GENERATED_DIR)/go.mod
	@echo "	google.golang.org/genproto/googleapis/rpc v0.0.0-20250707201910-8d1bb00bc6a7 // indirect" >> $(GENERATED_DIR)/go.mod
	@echo "	golang.org/x/net v0.38.0 // indirect" >> $(GENERATED_DIR)/go.mod
	@echo "	golang.org/x/sys v0.31.0 // indirect" >> $(GENERATED_DIR)/go.mod
	@echo "	golang.org/x/text v0.23.0 // indirect" >> $(GENERATED_DIR)/go.mod
	@echo ")" >> $(GENERATED_DIR)/go.mod
	@touch $(PROTO_TIMESTAMP_FILE)
	@echo "✅ protobuf 代码生成完成！"
	@echo "📁 生成的 Go 代码: ./$(GENERATED_DIR)/"

# 智能生成proto文件（仅在有变更时生成）  
gen-proto:
	@$(MAKE) check-proto-changed || $(MAKE) gen-proto-internal

# 强制重新生成proto文件
gen-proto-force:
	@echo "🔄 强制重新生成 protobuf 代码..."
	@$(MAKE) gen-proto-internal

# 清理proto生成的代码
proto-clean:
	@echo "清理proto生成的代码..."
	@rm -rf $(GENERATED_DIR)
	@rm -f $(PROTO_TIMESTAMP_FILE)
	@echo "proto代码清理完成"

# 交互式检查protobuf环境
check-proto-env:
	@echo "🔍 检查 protobuf 环境..."
	@echo "📍 protoc 版本:"
	@if command -v protoc >/dev/null 2>&1; then \
		protoc --version; \
	else \
		echo "  ❌ protoc 未安装"; \
	fi
	@echo "📍 Go protobuf 插件:"
	@for tool in protoc-gen-go protoc-gen-go-grpc protoc-gen-grpc-gateway protoc-gen-openapiv2; do \
		if command -v $$tool >/dev/null 2>&1; then \
			echo "  ✅ $$tool: 已安装"; \
		else \
			echo "  ❌ $$tool: 未安装"; \
		fi; \
	done
	@echo "📍 GOPATH: $(shell go env GOPATH)"
	@echo "📍 PATH 中是否包含 Go bin: $(if $(findstring $(shell go env GOPATH)/bin,$(PATH)),✅ 是,❌ 否)"

# 多平台构建
build-all: gen-proto
	@echo "构建所有平台版本..."
	@mkdir -p $(OUTPUT_DIR)
	@for os in linux darwin windows; do \
		for arch in amd64 arm64; do \
			echo "构建 $$os/$$arch..."; \
			ext=""; \
			if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
			CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
				go build -ldflags "$(LDFLAGS)" \
				-o $(OUTPUT_DIR)/$(BINARY_NAME)-$$os-$$arch$$ext ./cmd/$(BINARY_NAME); \
		done; \
	done
	@echo "所有平台构建完成！"

# 发布构建
release: gen-proto
	@echo "执行发布构建..."
	@./scripts/release.sh $(VERSION)

# 发布到GitHub
release-github: release
	@echo "发布到GitHub..."
	@if [ -z "$(GITHUB_TOKEN)" ]; then \
		echo "请设置GITHUB_TOKEN环境变量"; \
		exit 1; \
	fi
	@echo "使用GitHub CLI发布..."
	@gh release create $(VERSION) release/* --title "NSPass Agent $(VERSION)" --notes-file release/RELEASE_NOTES.md

# 安装到系统
install: build
	@echo "安装到系统..."
	@sudo cp $(BINARY_PATH) $(INSTALL_PATH)/
	@sudo chmod +x $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "安装完成"

# 从系统卸载
uninstall:
	@echo "从系统卸载..."
	@sudo rm -f $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "卸载完成"

# 运行程序
run: build
	@echo "运行 $(BINARY_NAME)..."
	@./$(BINARY_PATH) -c configs/config.yaml

# 显示帮助信息
help:
	@echo "📚 NSPass Agent Makefile 使用指南"
	@echo ""
	@echo "🛠️  开发命令:"
	@echo "  make run              - 运行开发服务器 (包含proto生成检查)"
	@echo "  make build            - 构建项目"
	@echo "  make build-all        - 构建所有平台版本"
	@echo "  make all              - 清理并重新构建（默认）"
	@echo ""
	@echo "📄 Proto & 代码生成:"
	@echo "  make gen-proto        - 智能生成protobuf代码 (仅在有变更时)"
	@echo "  make gen-proto-force  - 强制重新生成protobuf代码"
	@echo ""
	@echo "🔧 工具安装:"
	@echo "  make install-proto-tools - 安装/更新protobuf Go插件"
	@echo "  make check-proto-tools   - 检查protobuf工具安装状态"
	@echo "  make check-proto-env     - 检查protobuf环境"
	@echo ""
	@echo "🧹 清理 & 维护:"
	@echo "  make clean            - 清理构建文件"
	@echo "  make deep-clean       - 深度清理（包括生成代码和缓存）"
	@echo "  make proto-clean      - 清理proto生成的代码"
	@echo "  make deps             - 下载Go依赖"
	@echo "  make format           - 格式化Go代码"
	@echo "  make lint             - 检查Go代码"
	@echo ""
	@echo "🚀 发布相关:"
	@echo "  make release          - 构建发布版本"
	@echo "  make release-github   - 发布到GitHub (需要GITHUB_TOKEN)"
	@echo ""
	@echo "🧪 测试相关:"
	@echo "  make test             - 运行测试"
	@echo ""
	@echo "💻 系统相关:"
	@echo "  make install          - 安装到系统"
	@echo "  make uninstall        - 从系统卸载"
	@echo ""
	@echo "💡 提示: 运行 make run 会自动检查并安装必要的工具"