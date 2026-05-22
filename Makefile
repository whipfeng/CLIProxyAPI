# 统一二进制文件名
BINARY_NAME=cli-proxy

.PHONY: all run clean build-win build-mac-m build-mac-intel

# 默认编译全部
all: clean build-win build-mac-m build-mac-intel

# 调试运行：直接使用默认 Token
run:
	go run ./cmd/server

# 编译 Windows 版本
build-win:
	@echo "编译 Windows x64..."
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o ./bin/$(BINARY_NAME)-win-amd64.exe ./cmd/server

# 编译 macOS Arm 版本 (M1/M2/M3 芯片)
build-mac-m:
	@echo "编译 macOS Arm64..."
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o ./bin/$(BINARY_NAME)-mac-arm64 ./cmd/server

# 编译 macOS Intel 版本
build-mac-intel:
	@echo "编译 macOS Amd64..."
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o ./bin/$(BINARY_NAME)-mac-amd64 ./cmd/server

# 清理 bin 目录
clean:
	@rm -rf ./bin/*