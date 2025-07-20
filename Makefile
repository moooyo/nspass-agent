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

.PHONY: all build clean deep-clean test install uninstall deps lint format help proto-deps proto-gen gen-proto proto-clean build-all release release-github run

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
	@rm -rf $(GENERATED_DIR)
	@go clean -cache
	@go clean -modcache

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

# 安装proto依赖
proto-deps:
	@echo "检查proto依赖..."
	@if ! command -v protoc >/dev/null 2>&1; then \
		echo "错误: 请先安装 protoc"; \
		exit 1; \
	fi
	@if ! command -v protoc-gen-go >/dev/null 2>&1; then \
		echo "安装 protoc-gen-go..."; \
		go install google.golang.org/protobuf/cmd/protoc-gen-go@latest; \
	fi
	@if ! command -v protoc-gen-go-grpc >/dev/null 2>&1; then \
		echo "安装 protoc-gen-go-grpc..."; \
		go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest; \
	fi

# 生成proto代码
proto-gen: proto-deps
	@echo "生成proto代码..."
	@mkdir -p $(GENERATED_DIR)
	@echo "Generated目录: $(GENERATED_DIR)"
	@echo "找到的proto文件数量: $(shell find $(PROTO_DIR) -name '*.proto' | wc -l)"
	@export PATH="$$PATH:$(shell go env GOPATH)/bin"; \
	for proto in $(PROTO_FILES); do \
		echo "处理: $$proto"; \
		echo "执行命令: protoc --proto_path=$(PROTO_DIR) --proto_path=$(PROTO_DIR)/google/protobuf --go_out=$(GENERATED_DIR) --go_opt=paths=source_relative --go-grpc_out=$(GENERATED_DIR) --go-grpc_opt=paths=source_relative $$proto"; \
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
	@echo "proto代码生成完成！"

# 生成proto代码（别名）
gen-proto: proto-gen

# 清理proto生成的代码
proto-clean:
	@echo "清理proto生成的代码..."
	@rm -rf $(GENERATED_DIR)

# 多平台构建
build-all: proto-gen
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
release: proto-gen
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
	@echo "NSPass Agent Makefile 使用说明："
	@echo ""
	@echo "构建相关："
	@echo "  build        构建二进制文件"
	@echo "  build-all    构建所有平台版本"
	@echo "  all          清理并重新构建（默认）"
	@echo ""
	@echo "运行相关："
	@echo "  run          运行Agent程序"
	@echo ""
	@echo "发布相关："
	@echo "  release      构建发布版本"
	@echo "  release-github 发布到GitHub (需要GITHUB_TOKEN)"
	@echo ""
	@echo "测试相关："
	@echo "  test         运行测试"
	@echo ""
	@echo "清理相关："
	@echo "  clean        清理构建文件"
	@echo "  deep-clean   深度清理（包括生成代码和缓存）"
	@echo "  proto-clean  清理proto生成的代码"
	@echo ""
	@echo "开发相关："
	@echo "  lint         代码检查"
	@echo "  format       格式化代码"
	@echo "  deps         安装依赖"
	@echo ""
	@echo "Proto相关："
	@echo "  proto-deps   安装proto依赖"
	@echo "  proto-gen    生成proto代码"
	@echo "  gen-proto    生成proto代码（别名）"
	@echo ""
	@echo "系统相关："
	@echo "  install      安装到系统"
	@echo "  uninstall    从系统卸载"
	@echo ""
	@echo "其他："
	@echo "  help         显示此帮助信息"